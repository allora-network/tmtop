package logger

import (
	"io"
	"os"

	configPkg "main/pkg/config"

	bsync "github.com/brynbellomy/go-utils"
	"github.com/rs/zerolog"
)

func GetDefaultLogger() *zerolog.Logger {
	log := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	return &log
}

// Writer forwards zerolog output to a mailbox (non-blocking, so a
// slow log consumer can never freeze the app) and optionally tees to
// a debug file.
type Writer struct {
	io.Writer
	DebugFile *os.File
	Mailbox   *bsync.Mailbox[string]
}

func NewWriter(mb *bsync.Mailbox[string], config *configPkg.Config) Writer {
	writer := Writer{
		Mailbox: mb,
	}

	if config.DebugFile != "" {
		debugFile, err := os.OpenFile(config.DebugFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
		if err != nil {
			panic(err)
		}

		writer.DebugFile = debugFile
	}

	return writer
}

func (w Writer) Write(msg []byte) (int, error) {
	w.Mailbox.Deliver(string(msg))

	if w.DebugFile != nil {
		if _, err := w.DebugFile.Write(msg); err != nil {
			panic(err)
		}
		err := w.DebugFile.Sync()
		if err != nil {
			panic(err)
		}
	}

	return len(msg), nil
}

func GetLogger(mb *bsync.Mailbox[string], config *configPkg.Config) *zerolog.Logger {
	writer := zerolog.ConsoleWriter{
		Out:     NewWriter(mb, config),
		NoColor: true,
	}
	log := zerolog.New(writer).With().Timestamp().Logger()

	if config.Verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	}

	return &log
}
