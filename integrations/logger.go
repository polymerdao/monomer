package integrations

import (
	"context"
	"fmt"
	"log/slog"

	"cosmossdk.io/log"
	ethlog "github.com/ethereum/go-ethereum/log"
)

// cosmosToETHLogger adapts a Cosmos Logger to fulfill geth's logger interface.
type cosmosToETHLogger struct {
	log log.Logger
}

var _ ethlog.Logger = (*cosmosToETHLogger)(nil)

func (l *cosmosToETHLogger) With(kvs ...any) ethlog.Logger {
	return &cosmosToETHLogger{
		log: l.log.With(kvs...),
	}
}

func (l *cosmosToETHLogger) New(kvs ...any) ethlog.Logger {
	return l.With(kvs...)
}

func (l *cosmosToETHLogger) Log(level slog.Level, msg string, kvs ...any) {
	l.Write(level, msg, kvs...)
}

func (l *cosmosToETHLogger) Trace(msg string, kvs ...any) {
	l.log.Debug(msg, kvs...)
}

func (l *cosmosToETHLogger) Debug(msg string, kvs ...any) {
	l.log.Debug(msg, kvs...)
}

func (l *cosmosToETHLogger) Info(msg string, kvs ...any) {
	l.log.Info(msg, kvs...)
}

func (l *cosmosToETHLogger) Warn(msg string, kvs ...any) {
	l.log.Warn(msg, kvs...)
}

func (l *cosmosToETHLogger) Error(msg string, kvs ...any) {
	l.log.Error(msg, kvs...)
}

func (l *cosmosToETHLogger) Crit(msg string, kvs ...any) {
	l.log.Error(msg, kvs...)
	panic(msg) // TODO should it be os.Exit?
}

func (l *cosmosToETHLogger) Write(level slog.Level, msg string, kvs ...any) {
	switch level {
	case slog.LevelDebug:
		l.log.Debug(msg, kvs...)
	case slog.LevelInfo:
		l.log.Info(msg, kvs...)
	case slog.LevelWarn:
		l.log.Warn(msg, kvs...)
	case slog.LevelError:
		l.log.Error(msg, kvs...)
	default:
		panic(fmt.Errorf("unknown slog level: %s", level))
	}
}

func (l *cosmosToETHLogger) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (l *cosmosToETHLogger) Handler() slog.Handler {
	return nil
}
