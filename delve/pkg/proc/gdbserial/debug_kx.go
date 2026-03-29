package gdbserial

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	kxDebugserverLogDirEnv        = "HAPPYGO_KX_DEBUGSERVER_LOG_DIR"
	kxPacketPreviewLimit          = 256
	kxDebugserverRemoteLogBitmask = "LOG_PROCESS|LOG_THREAD|LOG_EXCEPTIONS|LOG_BREAKPOINTS|LOG_EVENTS|LOG_STEP|LOG_TASK|LOG_RNB_REMOTE|LOG_RNB_EVENTS|LOG_RNB_PROC|LOG_RNB_PACKETS"
)

type kxProcessInfo struct {
	createdAt           time.Time
	createdBy           string
	stubPID             int
	targetPath          string
	cmdline             string
	debugserverLogPath  string
	debugserverLogFlags string
	lastEvent           string
	lastEventAt         time.Time
}

var (
	kxProcessMu        sync.Mutex
	kxProcessInfoByPtr = map[*gdbProcess]*kxProcessInfo{}
)

func kxLogf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[happygo-kx][gdbserial][%s] %s\n", time.Now().UTC().Format(time.RFC3339Nano), fmt.Sprintf(format, args...))
}

func kxCaller(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "<unknown>"
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

func kxProcessSummary(p *gdbProcess, info *kxProcessInfo) string {
	stubPID := 0
	targetPath := ""
	createdBy := "<unknown>"
	if info != nil {
		stubPID = info.stubPID
		targetPath = info.targetPath
		createdBy = info.createdBy
	}
	targetPID := p.conn.pid
	targetBase := ""
	if targetPath != "" {
		targetBase = filepath.Base(targetPath)
	}
	if targetBase == "" {
		targetBase = "<unknown>"
	}
	return fmt.Sprintf("p=%p stubPID=%d targetPID=%d target=%q exited=%t detached=%t createdBy=%s", p, stubPID, targetPID, targetBase, p.exited, p.detached, createdBy)
}

func kxRegisterProcess(p *gdbProcess) {
	info := &kxProcessInfo{
		createdAt: time.Now().UTC(),
		createdBy: kxCaller(3),
	}
	if p.process != nil {
		info.stubPID = p.process.Pid
	}

	kxProcessMu.Lock()
	kxProcessInfoByPtr[p] = info
	kxProcessMu.Unlock()

	kxLogf("register %s", kxProcessSummary(p, info))
}

func kxSetProcessContext(p *gdbProcess, path, cmdline string) {
	kxProcessMu.Lock()
	if info := kxProcessInfoByPtr[p]; info != nil {
		if path != "" {
			info.targetPath = path
		}
		if cmdline != "" {
			info.cmdline = cmdline
		}
	}
	kxProcessMu.Unlock()
}

func kxTraceProcess(p *gdbProcess, format string, args ...any) {
	event := fmt.Sprintf(format, args...)

	kxProcessMu.Lock()
	info := kxProcessInfoByPtr[p]
	if info != nil {
		info.lastEvent = event
		info.lastEventAt = time.Now().UTC()
	}
	summary := kxProcessSummary(p, info)
	kxProcessMu.Unlock()

	kxLogf("%s %s", summary, event)
}

func kxUnregisterProcess(p *gdbProcess, reason string) {
	kxProcessMu.Lock()
	info := kxProcessInfoByPtr[p]
	delete(kxProcessInfoByPtr, p)
	summary := kxProcessSummary(p, info)
	kxProcessMu.Unlock()

	kxLogf("unregister %s reason=%s", summary, reason)
}

func kxSetDebugserverLogConfig(p *gdbProcess, path, flags string) {
	kxProcessMu.Lock()
	if info := kxProcessInfoByPtr[p]; info != nil {
		info.debugserverLogPath = path
		info.debugserverLogFlags = flags
	}
	kxProcessMu.Unlock()
}

func kxConnSummary(conn *gdbConn) string {
	return fmt.Sprintf("conn=%p pid=%d ack=%t running=%t multiprocess=%t threadSuffix=%t debugserver=%t logPath=%q", conn, conn.pid, conn.ack, conn.running, conn.multiprocess, conn.threadSuffixSupported, conn.isDebugserver, conn.kxDebugserverLogPath)
}

func kxTraceConn(conn *gdbConn, format string, args ...any) {
	kxLogf("%s %s", kxConnSummary(conn), fmt.Sprintf(format, args...))
}

func kxPacketPreview(pkt []byte, binary bool) string {
	if len(pkt) == 0 {
		return "<empty>"
	}
	out := pkt
	truncated := false
	if len(out) > kxPacketPreviewLimit {
		out = out[:kxPacketPreviewLimit]
		truncated = true
	}
	var preview string
	if binary {
		preview = hex.EncodeToString(out)
	} else {
		preview = strconv.QuoteToASCII(string(out))
	}
	if truncated {
		return fmt.Sprintf("%s...(len=%d)", preview, len(pkt))
	}
	return preview
}

func kxSanitizeFileComponent(s string) string {
	if s == "" {
		return "debugserver"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case 'a' <= r && r <= 'z':
			b.WriteRune(r)
		case 'A' <= r && r <= 'Z':
			b.WriteRune(r)
		case '0' <= r && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "debugserver"
	}
	return out
}

func kxPrepareDebugserverLogFile(target string) (string, error) {
	dir := os.Getenv(kxDebugserverLogDirEnv)
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, fmt.Sprintf("happygo-kx-debugserver-%s-*.log", kxSanitizeFileComponent(filepath.Base(target))))
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		return "", err
	}
	return path, nil
}

func LogKXProcessRegistry(reason string) {
	type entry struct {
		summary string
		info    kxProcessInfo
	}

	entries := []entry{}
	now := time.Now().UTC()

	kxProcessMu.Lock()
	for p, info := range kxProcessInfoByPtr {
		entries = append(entries, entry{
			summary: kxProcessSummary(p, info),
			info:    *info,
		})
	}
	kxProcessMu.Unlock()

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].info.createdAt.Before(entries[j].info.createdAt)
	})

	kxLogf("registry reason=%q live=%d", reason, len(entries))
	for _, entry := range entries {
		cmdline := entry.info.cmdline
		if cmdline == "" {
			cmdline = "<empty>"
		}
		lastEvent := entry.info.lastEvent
		if lastEvent == "" {
			lastEvent = "<none>"
		}
		lastAt := "<never>"
		if !entry.info.lastEventAt.IsZero() {
			lastAt = entry.info.lastEventAt.Format(time.RFC3339Nano)
		}
		debugserverLogPath := entry.info.debugserverLogPath
		if debugserverLogPath == "" {
			debugserverLogPath = "<none>"
		}
		debugserverLogFlags := entry.info.debugserverLogFlags
		if debugserverLogFlags == "" {
			debugserverLogFlags = "<none>"
		}
		kxLogf("registry live %s age=%s createdAt=%s lastAt=%s lastEvent=%q cmdline=%q debugserverLog=%q debugFlags=%q", entry.summary, now.Sub(entry.info.createdAt).Round(time.Millisecond), entry.info.createdAt.Format(time.RFC3339Nano), lastAt, lastEvent, cmdline, debugserverLogPath, debugserverLogFlags)
	}
}
