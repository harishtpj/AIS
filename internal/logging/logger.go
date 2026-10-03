package logging

import (
	"io"
	"os"

	"github.com/rs/zerolog"
	"golang.org/x/term"
)

func New() zerolog.Logger {
	writer := outputWriter()

	return zerolog.New(writer).
		With().
		Timestamp().
		Logger()
}

func outputWriter() io.Writer {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		return consoleWriter()
	}

	return os.Stdout
}

func consoleWriter() io.Writer {
	return zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "15:04:05",
		NoColor:    false,
	}
}
