package ctxlog

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
)

// Mode controls CLI verbosity and sub-process output routing.
// Three modes are supported:
//
//   - Info  (default): slog at INFO; sub-process stdout/stderr is captured and
//     only written to stderr when the command fails.
//   - Trace: slog at DEBUG; sub-process output passes through to the terminal
//     without adding extra verbose flags to sub-commands.
//   - Debug: slog at DEBUG; sub-process output passes through AND extra verbose
//     flags are added to sub-commands (e.g. k3d --verbose, kubectl --v=6).
type Mode int

const (
	// ModeInfo is the default: slog at INFO, sub-process output suppressed.
	ModeInfo Mode = iota
	// ModeTrace enables slog DEBUG without extra sub-command verbose flags.
	ModeTrace
	// ModeDebug enables slog DEBUG and activates verbose flags on sub-commands.
	ModeDebug
)

// ParseMode converts a flag string ("info", "trace", "debug") to a Mode.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "info":
		return ModeInfo, nil
	case "trace":
		return ModeTrace, nil
	case "debug":
		return ModeDebug, nil
	default:
		return ModeInfo, fmt.Errorf("unknown log level %q: must be info, trace, or debug", s)
	}
}

// SlogLevel returns the slog.Level that corresponds to this mode.
func (m Mode) SlogLevel() slog.Level {
	if m == ModeInfo {
		return slog.LevelInfo
	}
	return slog.LevelDebug
}

// VerboseSubprocesses reports whether verbose flags should be added to
// sub-commands. True only at ModeDebug.
func (m Mode) VerboseSubprocesses() bool { return m == ModeDebug }

type modeKey struct{}

// WithMode returns a new context carrying m.
func WithMode(ctx context.Context, m Mode) context.Context {
	return context.WithValue(ctx, modeKey{}, m)
}

// GetMode returns the Mode stored in ctx, defaulting to ModeInfo.
func GetMode(ctx context.Context) Mode {
	if m, ok := ctx.Value(modeKey{}).(Mode); ok {
		return m
	}
	return ModeInfo
}

// RunCmd executes cmd, routing output based on the log mode in ctx.
//   - ModeInfo: captures stdout+stderr; writes them to os.Stderr only if the
//     command fails, so the terminal stays clean on success.
//   - ModeTrace / ModeDebug: streams stdout+stderr directly to the terminal.
func RunCmd(ctx context.Context, cmd *exec.Cmd) error {
	if GetMode(ctx) == ModeInfo {
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		if err := cmd.Run(); err != nil {
			if buf.Len() > 0 {
				_, _ = os.Stderr.Write(buf.Bytes())
			}
			return err
		}
		return nil
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
