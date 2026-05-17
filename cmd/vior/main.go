// Vior CLI entry point.
package main

import (
	"os"

	"github.com/subhashraveendran/vior/cmd/vior/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
