// Package tcpstdio implements the private `agentctl tcp-stdio` bridge used by
// the Kubernetes exec transport. The backend opens one pods/exec stream per
// forwarded connection and runs this bridge in the Pod, so agentctl stays bound
// to Pod loopback.
//
// The package depends only on the standard library so agentctl and the
// backend's Kubernetes executor can share the subcommand name without pulling
// in each other's dependencies.
package tcpstdio

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// Subcommand is the agentctl subcommand that runs the bridge.
const Subcommand = "tcp-stdio"

const dialTimeout = 10 * time.Second

// Run parses `--port N`, dials 127.0.0.1:N, and bridges stdin and stdout to
// that connection. It returns the process exit code: 0 after a clean bridge,
// 1 for a dial or copy failure, and 2 for invalid arguments.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	port, err := ParseArgs(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "agentctl %s: %v\n", Subcommand, err)
		return 2
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", address, dialTimeout)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "agentctl %s: dial %s: %v\n", Subcommand, address, err)
		return 1
	}
	if err := Bridge(conn, stdin, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "agentctl %s: %v\n", Subcommand, err)
		return 1
	}
	return 0
}

// ParseArgs accepts exactly `--port N` with N in 1..65535. The bridge never
// takes a host: it dials only Pod loopback.
func ParseArgs(args []string, stderr io.Writer) (int, error) {
	flags := flag.NewFlagSet(Subcommand, flag.ContinueOnError)
	flags.SetOutput(stderr)
	port := flags.Int("port", 0, "TCP port on 127.0.0.1 to bridge")
	if err := flags.Parse(args); err != nil {
		return 0, err
	}
	if flags.NArg() > 0 {
		return 0, fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if *port < 1 || *port > 65535 {
		return 0, errors.New("--port must be between 1 and 65535")
	}
	return *port, nil
}

// Bridge copies stdin to conn and conn to stdout, and closes conn on return.
//
// The two directions end independently, like kubectl port-forward. EOF on
// stdin means the client finished its request: the bridge half-closes the TCP
// write side so the server sees EOF, and keeps copying the response until the
// server closes the connection. There is no drain deadline, so a slow or
// chunked response after the request ends is delivered in full. The bridge
// returns when the TCP peer closes (EOF on conn) or a write to stdout fails.
func Bridge(conn net.Conn, stdin io.Reader, stdout io.Writer) error {
	defer func() { _ = conn.Close() }()
	go func() {
		_, _ = io.Copy(conn, stdin)
		if halfCloser, ok := conn.(interface{ CloseWrite() error }); ok {
			_ = halfCloser.CloseWrite()
		}
	}()
	_, err := io.Copy(stdout, conn)
	if err == nil || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}
