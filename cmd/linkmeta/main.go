package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is injected at build time via -ldflags "-X main.version=<v>".
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:     "linkmeta",
		Short:   "Self-hosted URL metadata extraction service",
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd)
		},
	}
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run is implemented in the server bootstrap task. Temporary stub.
func run(cmd *cobra.Command) error {
	fmt.Println("linkmeta", version)
	return nil
}
