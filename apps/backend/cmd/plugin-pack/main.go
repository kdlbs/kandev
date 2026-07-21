package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run parses CLI flags and packs -dir into -out. Returns a process exit
// code (0 success, 1 packaging failure, 2 usage error).
func run(args []string) int {
	fs := flag.NewFlagSet("plugin-pack", flag.ContinueOnError)
	dir := fs.String("dir", "", "plugin package directory (must contain manifest.yaml)")
	out := fs.String("out", "", "output tar.gz path")
	platformOnly := fs.Bool("platform-only", false,
		"include only the current host platform's server/ executable (smaller test packages)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dir == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "plugin-pack: -dir and -out are required")
		fs.Usage()
		return 2
	}

	if err := packToFile(*dir, *out, *platformOnly); err != nil {
		fmt.Fprintln(os.Stderr, "plugin-pack:", err)
		return 1
	}
	_, _ = fmt.Fprintf(os.Stdout, "plugin-pack: wrote %s\n", *out)
	return 0
}
