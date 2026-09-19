// Command urbino is the Urbino gateway entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"example.com/urbino/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	workDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "urbino: cannot determine working directory: %v\n", err)
		os.Exit(cli.ExitError)
	}

	environment := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(name, "URBINO_") {
			environment[name] = value
		}
	}
	code := cli.Run(ctx, cli.Options{
		Args:        os.Args[1:],
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		LookupEnv:   os.LookupEnv,
		Environment: environment,
		WorkDir:     workDir,
	})
	os.Exit(code)
}
