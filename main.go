package main

import (
	"os"

	"github.com/12345nikhilkumars/crictui/cmd"
)

var version = "dev"

// main is the entry point of the application
func main() {
	cmd.SetVersion(version)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
