package main

import (
	"os"

	"taskman/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
