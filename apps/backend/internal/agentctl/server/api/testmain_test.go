package api

import (
	"fmt"
	"net"
	"os"
	"testing"

	"go.uber.org/goleak"
)

const apiTestVscodeFixtureEnv = "KANDEV_API_TEST_VSCODE_FIXTURE"

func TestMain(m *testing.M) {
	if os.Getenv(apiTestVscodeFixtureEnv) != "" {
		runAPITestVscodeFixture()
		return
	}
	goleak.VerifyTestMain(m)
}

func runAPITestVscodeFixture() {
	address := ""
	for i, arg := range os.Args[:len(os.Args)-1] {
		if arg == "--bind-addr" {
			address = os.Args[i+1]
			break
		}
	}
	if address == "" {
		fmt.Fprintln(os.Stderr, "VS Code fixture: missing --bind-addr")
		os.Exit(2)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "VS Code fixture: listen: %v\n", err)
		os.Exit(2)
	}
	defer func() { _ = listener.Close() }()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		_ = conn.Close()
	}
}
