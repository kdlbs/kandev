// kandev-sdk-check verifies installed SDK compatibility without changing a project.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kdlbs/kandev/pluginsdk/internal/compatibility"
)

func run(args []string, directory string, out, errOut io.Writer) int {
	options := compatibility.Options{Directory: directory}
	flags := flag.NewFlagSet("kandev-sdk-check", flag.ContinueOnError)
	flags.SetOutput(errOut)
	flags.StringVar(&options.Manifest, "manifest", "manifest.yaml", "plugin manifest path")
	flags.StringVar(&options.FrontendSDK, "frontend-sdk", "", "installed node_modules/@kdlbs/plugin-sdk path")
	flags.BoolVar(&options.FrontendOnly, "frontend-only", false, "check a frontend-only plugin without a Go dependency")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || (options.FrontendOnly && options.FrontendSDK == "") {
		_, _ = fmt.Fprintln(errOut, "unexpected arguments or --frontend-only without --frontend-sdk")
		return 2
	}
	records, err := compatibility.CheckProject(options)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "SDK compatibility:", err)
		return 1
	}
	for _, record := range records {
		_, _ = fmt.Fprintf(out, "SDK %s: compatible (minimum Kandev %s)\n", record.SDKVersion, record.Minimum)
	}
	return 0
}
func main() {
	directory, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(run(os.Args[1:], directory, os.Stdout, os.Stderr))
}
