package acp

import (
	"io"
	"log/slog"

	acpsdk "github.com/coder/acp-go-sdk"
)

// NewClientSideConnectionWithLogger creates an ACP client connection and
// installs its logger before the SDK's receive goroutine can read from the
// peer. The SDK starts that goroutine in NewClientSideConnection, while
// SetLogger is not synchronized with it.
func NewClientSideConnectionWithLogger(
	client acpsdk.Client,
	peerInput io.Writer,
	peerOutput io.Reader,
	logger *slog.Logger,
	opts ...acpsdk.ConnectionOption,
) *acpsdk.ClientSideConnection {
	output := &loggerReadyReader{
		ready:  make(chan struct{}),
		reader: peerOutput,
	}
	conn := acpsdk.NewClientSideConnection(client, peerInput, output, opts...)
	if logger != nil {
		conn.SetLogger(logger)
	}
	close(output.ready)
	return conn
}

type loggerReadyReader struct {
	ready  chan struct{}
	reader io.Reader
}

func (r *loggerReadyReader) Read(p []byte) (int, error) {
	<-r.ready
	return r.reader.Read(p)
}
