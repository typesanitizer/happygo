package gdbserial

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"
)

type kxProcessInfo struct {
	createdAt   time.Time
	createdBy   string
	stubPID     int
	targetPath  string
	cmdline     string
	lastEvent   string
	lastEventAt time.Time
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
		kxLogf("registry live %s age=%s createdAt=%s lastAt=%s lastEvent=%q cmdline=%q", entry.summary, now.Sub(entry.info.createdAt).Round(time.Millisecond), entry.info.createdAt.Format(time.RFC3339Nano), lastAt, lastEvent, cmdline)
	}
}
