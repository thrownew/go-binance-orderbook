package binanceorderbook

import (
	"fmt"
	"io"
	"log/slog"
)

var _ Logger = NopLogger{}

type NopLogger struct{}

func (NopLogger) Debugf(string, ...any) {}
func (NopLogger) Infof(string, ...any)  {}
func (NopLogger) Warnf(string, ...any)  {}
func (NopLogger) Errorf(string, ...any) {}

type SlogLogger struct {
	l *slog.Logger
}

func NewSlogLogger(l *slog.Logger) SlogLogger {
	if l == nil {
		l = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return SlogLogger{l: l}
}

func (a SlogLogger) Debugf(format string, args ...any) {
	a.l.Debug(fmt.Sprintf(format, args...))
}

func (a SlogLogger) Infof(format string, args ...any) {
	a.l.Info(fmt.Sprintf(format, args...))
}

func (a SlogLogger) Warnf(format string, args ...any) {
	a.l.Warn(fmt.Sprintf(format, args...))
}

func (a SlogLogger) Errorf(format string, args ...any) {
	a.l.Error(fmt.Sprintf(format, args...))
}
