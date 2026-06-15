package cli

const helpText = `kandev launcher

Usage:
  kandev run [--port <port>] [--verbose] [--debug]
  kandev dev [--port <port>]
  kandev start [--port <port>] [--verbose] [--debug]
  kandev [--port <port>] [--verbose] [--debug]
  kandev --dev [--port <port>]
  kandev service install|uninstall|start|stop|restart|status|logs [--system]

Options:
  dev              Use local repo for dev if available.
  start            Use local production build.
  run              Use installed runtime bundle (default).
  service          Manage kandev as an OS service.
  --dev            Alias for "dev".
  --version, -V    Print CLI version and exit.
  --port           Port for the Go backend. Alias for --backend-port.
  --verbose, -v    Show info logs from backend + web.
  --debug          Show debug logs + agent message dumps.
  --headless       Skip opening the browser. Used by service units.
  --help, -h       Show help.

Advanced:
  --backend-port         Same as --port.
  --web-internal-port    Override the internal Next.js port.
  --web-port             Deprecated alias for --web-internal-port.
`

func Help() string {
	return helpText
}
