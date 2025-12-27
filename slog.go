//go:build go1.21
// +build go1.21

package binanceorderbook

import (
	"fmt"
	"log/slog"
)

type SlogLogger struct {
	l *slog.Logger
}

func NewSlogLogger(l *slog.Logger) SlogLogger {
	if l == nil {
		l = slog.Default().With("component", "binance-orderbook")
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
