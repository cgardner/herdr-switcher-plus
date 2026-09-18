// Command gendocs writes the reference tables the registries define into the
// marked regions of the documentation.
//
// Run it with `make docs`. With -check it writes nothing and exits non-zero
// when a file is out of date, which is how CI catches documentation that was
// not regenerated. The work lives in internal/docs so it can be tested.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cgardner/herdr-switcher-plus/internal/docs"
)

func main() {
	check := flag.Bool("check", false, "report stale files instead of rewriting them")
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	stale, err := docs.Generate(*root, *check)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gendocs:", err)
		os.Exit(1)
	}
	for _, path := range stale {
		if *check {
			fmt.Fprintf(os.Stderr, "gendocs: %s is out of date; run `make docs`\n", path)
		} else {
			fmt.Println("updated", path)
		}
	}
	if *check && len(stale) > 0 {
		os.Exit(1)
	}
}
