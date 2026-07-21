package protocol

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	// maxHeaderLineBytes is generous for the two short headers defined by LSP,
	// and prevents an unterminated header from growing without bound.
	maxHeaderLineBytes = 8 << 10
	// maxHeaderBytes allows several extension headers while bounding the total
	// work spent before the mandatory blank-line delimiter.
	maxHeaderBytes = 32 << 10
	// MaxMessageBytes caps each JSON-RPC frame at 16 MiB. LSP bodies can contain
	// whole documents and large semantic-token responses, while a bounded frame
	// keeps a single language server from forcing an arbitrary allocation.
	MaxMessageBytes = 16 << 20
)

// ReadMessage reads one LSP stdio message and returns the JSON-RPC body.
func ReadMessage(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	headerBytes := 0
	for {
		line, lineBytes, err := readHeaderLine(reader)
		if err != nil {
			return nil, err
		}
		headerBytes += lineBytes
		if headerBytes > maxHeaderBytes {
			return nil, fmt.Errorf("LSP headers exceed %d bytes", maxHeaderBytes)
		}
		if line == "" {
			break
		}
		if after, found := strings.CutPrefix(line, "Content-Length: "); found {
			n, err := strconv.Atoi(after)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("invalid Content-Length: %s", after)
			}
			contentLength = n
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	if contentLength > MaxMessageBytes {
		return nil, fmt.Errorf("Content-Length exceeds %d bytes: %d", MaxMessageBytes, contentLength)
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, err
	}

	return body, nil
}

func readHeaderLine(reader *bufio.Reader) (string, int, error) {
	var line []byte
	lineBytes := 0
	for {
		fragment, err := reader.ReadSlice('\n')
		lineBytes += len(fragment)
		if lineBytes > maxHeaderLineBytes {
			return "", lineBytes, fmt.Errorf("LSP header line exceeds %d bytes", maxHeaderLineBytes)
		}
		line = append(line, fragment...)
		if err == nil {
			return strings.TrimRight(string(line), "\r\n"), lineBytes, nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return "", lineBytes, err
		}
	}
}
