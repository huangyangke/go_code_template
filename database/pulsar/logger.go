package pulsar

import (
	"context"

	pulsarlog "github.com/apache/pulsar-client-go/pulsar/log"
	"github.com/sirupsen/logrus"

	"github.com/huangyangke/go-aikit/log"
)

type logHook struct{}

// defaultLogger creates a pulsar.Logger that bridges Pulsar client internal
// logs into the aikit structured logger.
func defaultLogger() pulsarlog.Logger {
	l := logrus.New()
	l.SetFormatter(&logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	})
	l.AddHook(&logHook{})
	return pulsarlog.NewLoggerWithLogrus(l)
}

func (h *logHook) Fire(entry *logrus.Entry) error {
	args := make([]log.D, 0, len(entry.Data)+1)
	args = append(args, log.KVString("log", entry.Message))
	for k, v := range entry.Data {
		args = append(args, log.KV(k, v))
	}
	ctx := context.Background()
	switch entry.Level {
	case logrus.PanicLevel, logrus.FatalLevel:
		log.Fatalv(ctx, args...)
	case logrus.ErrorLevel:
		log.Errorv(ctx, args...)
	case logrus.WarnLevel:
		log.Warnv(ctx, args...)
	case logrus.InfoLevel:
		log.Infov(ctx, args...)
	case logrus.DebugLevel, logrus.TraceLevel:
		log.Debugv(ctx, args...)
	}
	return nil
}

func (h *logHook) Levels() []logrus.Level {
	return logrus.AllLevels
}
