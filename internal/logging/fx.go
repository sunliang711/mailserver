package logging

import (
	"github.com/rs/zerolog"
	"go.uber.org/fx/fxevent"
)

type FxLogger struct {
	logger zerolog.Logger
}

func NewFxLogger(logger zerolog.Logger) fxevent.Logger {
	return &FxLogger{
		logger: logger.With().Str("component", "fx").Logger(),
	}
}

func (l *FxLogger) LogEvent(event fxevent.Event) {
	switch e := event.(type) {
	case *fxevent.OnStartExecuted:
		if e.Err != nil {
			l.logger.Error().
				Err(e.Err).
				Str("function", e.FunctionName).
				Str("caller", e.CallerName).
				Dur("runtime", e.Runtime).
				Msg("fx on start hook failed")
			return
		}

		l.logger.Info().
			Str("function", e.FunctionName).
			Str("caller", e.CallerName).
			Dur("runtime", e.Runtime).
			Msg("fx on start hook completed")
	case *fxevent.OnStopExecuted:
		if e.Err != nil {
			l.logger.Error().
				Err(e.Err).
				Str("function", e.FunctionName).
				Str("caller", e.CallerName).
				Dur("runtime", e.Runtime).
				Msg("fx on stop hook failed")
			return
		}

		l.logger.Info().
			Str("function", e.FunctionName).
			Str("caller", e.CallerName).
			Dur("runtime", e.Runtime).
			Msg("fx on stop hook completed")
	case *fxevent.Invoked:
		if e.Err != nil {
			l.logger.Error().
				Err(e.Err).
				Str("function", e.FunctionName).
				Str("trace", e.Trace).
				Msg("fx invoke failed")
		}
	case *fxevent.RollingBack:
		l.logger.Error().Err(e.StartErr).Msg("fx start failed, rolling back")
	case *fxevent.RolledBack:
		if e.Err != nil {
			l.logger.Error().Err(e.Err).Msg("fx rollback failed")
			return
		}

		l.logger.Info().Msg("fx rollback completed")
	case *fxevent.Started:
		if e.Err != nil {
			l.logger.Error().Err(e.Err).Msg("fx app failed to start")
			return
		}

		l.logger.Info().Msg("fx app started")
	case *fxevent.Stopping:
		l.logger.Info().Str("signal", e.Signal.String()).Msg("fx app stopping")
	case *fxevent.Stopped:
		if e.Err != nil {
			l.logger.Error().Err(e.Err).Msg("fx app stopped with error")
			return
		}

		l.logger.Info().Msg("fx app stopped")
	case *fxevent.LoggerInitialized:
		if e.Err != nil {
			l.logger.Error().
				Err(e.Err).
				Str("constructor", e.ConstructorName).
				Msg("fx logger initialization failed")
		}
	}
}
