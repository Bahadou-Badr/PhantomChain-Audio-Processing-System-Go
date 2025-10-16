package logging

import (
	"go.uber.org/zap"
)

var Logger *zap.Logger

// Init initializes the global logger. dev=true uses development config.
func Init(dev bool) error {
	var err error
	if dev {
		cfg := zap.NewDevelopmentConfig()
		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.MessageKey = "msg"
		Logger, err = cfg.Build()
	} else {
		cfg := zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.MessageKey = "msg"
		Logger, err = cfg.Build()
	}
	return err
}
