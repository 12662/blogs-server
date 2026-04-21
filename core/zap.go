package core

import (
	"log"
	"os"
	"server/global"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func InitLogger() *zap.Logger {
	zapCfg := global.Config.Zap

	writerSyncer := getLogWriter(zapCfg.Filename, zapCfg.MaxSize, zapCfg.MaxBackups, zapCfg.MaxAge)

	if zapCfg.IsConsolePrint {
		writerSyncer = zapcore.NewMultiWriteSyncer(writerSyncer, zapcore.AddSync(os.Stdout))
	}

	encoder := getEncoder()

	var logLevel zapcore.Level
	if err := logLevel.UnmarshalText([]byte(zapCfg.Level)); err != nil {
		log.Fatalf("Failed to parse log level: %v", err)
	}

	//创建核心和日志实例
	core := zapcore.NewCore(encoder, writerSyncer, logLevel)
	logger := zap.New(core, zap.AddCaller())
	return logger

}

func getLogWriter(filename string, maxSize, maxBackups, maxAge int) zapcore.WriteSyncer {
	lumberjackLogger := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
	}

	return zapcore.AddSync(lumberjackLogger)
}

// getEncoder 返回一个为生产日志配置的 JSON 编码器
func getEncoder() zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeDuration = zapcore.SecondsDurationEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	return zapcore.NewJSONEncoder(encoderConfig)
}
