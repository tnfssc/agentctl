// Command cliproxy is the cliproxy command line tool.
package main

import (
	"context"
	"os"

	"github.com/tnfssc/cliproxy-cli/internal/cli"
)

func main() {
	// fang prints the error, styled, before Execute returns it.
	if err := cli.Execute(context.Background()); err != nil {
		os.Exit(1)
	}
}
