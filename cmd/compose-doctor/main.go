// Command compose-doctor diagnoses Docker Compose projects offline.
package main

import (
	"os"

	"github.com/tatitoxt/compose-doctor/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr, os.Environ()))
}
