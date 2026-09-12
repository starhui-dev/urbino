package securitylog

import (
	"log/slog"
)

// Logger 只接受预定义的安全字段，避免把凭据或请求正文写入日志。
type Logger struct{ logger *slog.Logger }

func New(logger *slog.Logger) Logger { return Logger{logger: logger} }

func (l Logger) Event(event, address string) {
	l.logger.Info("security_event", "event", event, "address", address)
}

func (l Logger) Error(event string, err error) {
	l.logger.Error("security_event", "event", event, "error_type", errorType(err))
}

func errorType(err error) string {
	if err == nil {
		return ""
	}
	return "runtime_error"
}
