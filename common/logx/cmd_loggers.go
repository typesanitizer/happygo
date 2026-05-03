// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package logx

import (
	"bytes"
	"fmt"
	"io"
)

// CmdLoggers returns line-oriented writers for command stdout and stderr streams.
func (ctx LogCtx) CmdLoggers(command fmt.Stringer) (io.Writer, io.Writer) {
	cmdLogger := ctx.With("cmd", command)

	stdout := io.Discard
	if ctx.IsDebugEnabled() {
		stdoutLogger := cmdLogger.With("stream", "stdout")
		stdout = newLineLogger(func(msg []byte) {
			stdoutLogger.Debug(string(msg))
		})
	}
	stderrLogger := cmdLogger.With("stream", "stderr")
	stderr := io.Writer(newLineLogger(func(msg []byte) {
		stderrLogger.Info(string(msg))
	}))

	return stdout, stderr
}

// FlushLogWriter flushes a log writer when it supports Flush.
func FlushLogWriter(w io.Writer) {
	if flusher, ok := w.(interface{ Flush() }); ok {
		flusher.Flush()
	}
}

type lineLogger struct {
	logLine func([]byte)
	buf     bytes.Buffer
}

func newLineLogger(logLine func([]byte)) *lineLogger {
	return &lineLogger{logLine: logLine, buf: bytes.Buffer{}}
}

func (l *lineLogger) Write(p []byte) (int, error) {
	// Can technically return io.ErrTooLarge on potential OOM
	if n, err := l.buf.Write(p); err != nil {
		return n, err
	}
	for {
		idx := bytes.IndexByte(l.buf.Bytes(), '\n')
		if idx < 0 {
			break
		}
		line := l.buf.Next(idx)
		_ = l.buf.Next(1) // consume newline
		l.logLine(bytes.TrimSuffix(line, []byte("\r")))
	}
	return len(p), nil
}

func (l *lineLogger) Flush() {
	if l.buf.Len() == 0 {
		return
	}
	l.logLine(bytes.TrimSuffix(l.buf.Bytes(), []byte("\r")))
	l.buf.Reset()
}
