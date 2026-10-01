package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is replaced by the full CLI in Task 8.
func run(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "relcheck: no subcommands yet")
	return 2
}
