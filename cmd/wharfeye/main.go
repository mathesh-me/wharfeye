package main

import (
	"os"

	"github.com/mathesh-me/wharfeye/cmd/wharfeye/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
