// Command agentctl switches coding agents between their own login and CLIProxyAPI servers.
package main

import (
	"context"
	"os"

	"github.com/tnfssc/agentctl/internal/cli"
)

func main() {
	// fang prints the error, styled, before Execute returns it.
	if err := cli.Execute(context.Background()); err != nil {
		os.Exit(1)
	}
}
