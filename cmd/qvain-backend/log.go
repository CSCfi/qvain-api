package main

import (
	"os"

	"github.com/NatLibFi/qvain-api/caller"
	"github.com/rs/zerolog"
)

type LocationHook struct {
	name          string
	stackInfoFunc func() string
}

func NewLocationHook(name string) *LocationHook {
	return &LocationHook{name: name, stackInfoFunc: caller.CreateStackInfoFunc(4, true)}
}

func (h LocationHook) Run(e *zerolog.Event, l zerolog.Level, msg string) {
	e.Str(h.name, h.stackInfoFunc())
}

// createAppLogger returns a configured logger with or without debugging output.
func createAppLogger(isDebugging bool) (logger zerolog.Logger) {
	zerolog.MessageFieldName = "msg"
	if isDebugging {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Hook(NewLocationHook("at")).With().Timestamp().Logger()
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}
	return logger
}
