package launcher

import (
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
		return runService(args[1:])
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
	switch opts.Command {
	case CommandStart:
		return runStart(opts)
	case CommandRun:
		return runInstalled(opts)
	}
	fmt.Fprintf(os.Stderr, "[kandev] native launcher command %q is not implemented yet\n", opts.Command)
	return 1
}
