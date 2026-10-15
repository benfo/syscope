package main

import (
	"os"

	"github.com/benfo/syscope/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
