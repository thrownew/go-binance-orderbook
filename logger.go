package binanceorderbook

import (
	"log"
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

func (a LogLogger) Debugf(format string, args ...any) { a.l.Printf("[DEBUG] "+format, args...) }
func (a LogLogger) Infof(format string, args ...any)  { a.l.Printf("[INFO] "+format, args...) }
func (a LogLogger) Warnf(format string, args ...any)  { a.l.Printf("[WARN] "+format, args...) }
func (a LogLogger) Errorf(format string, args ...any) { a.l.Printf("[ERROR] "+format, args...) }
