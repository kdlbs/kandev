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
  dev              Use local repo for dev (Go backend + Vite dev server).
  start            Use local production build.
  run              Use installed runtime bundle (default).
  service          Manage kandev as an OS service.
  --dev            Alias for "dev".
  --version, -V    Print CLI version and exit.
  --port           Port for the Go backend. Alias for --backend-port.
  --verbose, -v    Show info logs on stdout (also retained in the backend file).
  --debug          Retain debug logs in the backend file + agent message dumps.
  --headless       Skip opening the browser. Used by service units.
  --help, -h       Show help.

Advanced:
  --backend-port         Same as --port.
  --web-internal-port    Override the internal dev web port. The Go backend
                         reverse-proxies to it in dev; start/run serve from Go.
                         Also reads KANDEV_WEB_PORT.
`

func Help() string {
	return helpText
}
