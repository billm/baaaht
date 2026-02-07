package main

import (
	"os"

	"github.com/baaaht/orchestrator/cmd"
)

func main() {
	// Execute the root command
	cmd.Execute()

	// Ensure clean exit
	os.Exit(0)
}
