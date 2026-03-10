// Package logx provides a configured structured logger.
// All logging in this project should use a logger obtained from this package.
package logx

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	charmlog "github.com/charmbracelet/log" //nolint:depguard // logx is the designated wrapper
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// Logger is a structured logger.
type Logger = charmlog.Logger

// Re-exported log levels so callers don't need to import charmbracelet/log.
const (
	DebugLevel = charmlog.DebugLevel
	InfoLevel  = charmlog.InfoLevel
	WarnLevel  = charmlog.WarnLevel
	ErrorLevel = charmlog.ErrorLevel
	FatalLevel = charmlog.FatalLevel
)

// ColorSupport controls whether the logger emits ANSI colors.
type ColorSupport int

const (
	ColorSupport_Enable     ColorSupport = iota + 1
	ColorSupport_AutoDetect              // detect based on whether w is a TTY
	ColorSupport_Disable
)

// NewLogger creates a configured logger writing to w.
func NewLogger(w io.Writer, cs ColorSupport) *Logger {
	color := false
	switch cs {
	case ColorSupport_Enable:
		color = true
	case ColorSupport_AutoDetect:
		if f, ok := w.(*os.File); ok {
			color = term.IsTerminal(int(f.Fd()))
		}
	case ColorSupport_Disable:
		color = false
	}
	logger := charmlog.NewWithOptions(w, charmlog.Options{ //nolint:exhaustruct // only overriding what we need
		ReportTimestamp: true,
		Level:           charmlog.InfoLevel,
	})

	type levelDef struct {
		level charmlog.Level
		name  string
		fg    string // ANSI color number, only used when color=true
	}
	levels := []levelDef{
		{charmlog.DebugLevel, "DEBUG", "63"},
		{charmlog.InfoLevel, "INFO", "86"},
		{charmlog.WarnLevel, "WARN", "192"},
		{charmlog.ErrorLevel, "ERROR", "204"},
		{charmlog.FatalLevel, "FATAL", "134"},
	}

	styles := charmlog.DefaultStyles()
	for _, l := range levels {
		s := lipgloss.NewStyle().SetString(l.name).MaxWidth(5)
		if color {
			s = s.Bold(true).Foreground(lipgloss.Color(l.fg))
		}
		styles.Levels[l.level] = s
	}
	logger.SetStyles(styles)

	if color {
		logger.SetColorProfile(termenv.ANSI256)
	} else {
		logger.SetColorProfile(termenv.Ascii)
	}

	return logger
}
