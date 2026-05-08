package log

import "github.com/yosi-hq/go-backend-kit/logger"

type Logger = logger.Logger
type SlogLogger = logger.SlogLogger
type Option = logger.Option

var (
	New             = logger.New
	NewWithSlog     = logger.NewWithSlog
	ParseLevel      = logger.ParseLevel
	WithAttrs       = logger.WithAttrs
	WithLevel       = logger.WithLevel
	WithLevelString = logger.WithLevelString
	WithOutput      = logger.WithOutput
	WithTextHandler = logger.WithTextHandler
)
