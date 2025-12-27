package binanceorderbook

import (
	"fmt"
	"log"
	"log/slog"
)

var _ Logger = NopLogger{}

type NopLogger struct{}

func (NopLogger) Debugf(string, ...any) {}
func (NopLogger) Infof(string, ...any)  {}
func (NopLogger) Warnf(string, ...any)  {}
func (NopLogger) Errorf(string, ...any) {}

type LogLogger struct {
	l *log.Logger
}

func NewLogLogger(l *log.Logger) LogLogger {
	if l == nil {
		l = log.Default()
	}
	return LogLogger{l: l}
}

func (a LogLogger) Debugf(format string, args ...any) {
	a.l.Printf("[DEBUG] "+format, args...)
}

func (a LogLogger) Infof(format string, args ...any) {
	a.l.Printf("[INFO] "+format, args...)
}

func (a LogLogger) Warnf(format string, args ...any) {
	a.l.Printf("[WARN] "+format, args...)
}

func (a LogLogger) Errorf(format string, args ...any) {
	a.l.Printf("[ERROR] "+format, args...)
}

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
