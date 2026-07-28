package launcher

import (
	"context"
	"fmt"
	"os"

	"github.com/kandev/kandev/internal/launcher/cli"
)

type BuildInfo struct {
	Version   string
	Commit    string
	BuildTime string
}

func Run(args []string, build BuildInfo) int {
	if len(args) > 0 && args[0] == "service" {
		return runService(args[1:], build)
	}
	opts, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 2
	}
	if opts.ShowVersion {
		fmt.Println(build.Version)
		return 0
	}
	if opts.ShowHelp {
		fmt.Print(cli.Help())
		return 0
	}
	// Root context for the launcher process; bounds the backend health probe so
	// an unresponsive backend cannot hang the launcher indefinitely.
	ctx := context.Background()
	switch opts.Command {
	case CommandStart:
		return runStart(ctx, opts)
	case CommandRun:
		return runInstalled(ctx, opts)
	}
	fmt.Fprintf(os.Stderr, "[kandev] native launcher command %q is not implemented yet\n", opts.Command)
	return 1
}
