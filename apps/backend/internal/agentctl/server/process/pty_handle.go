package process

import "io"

// PtyHandle abstracts PTY operations across Unix and Windows.
// On Unix, this wraps creack/pty (*os.File).
// On Windows, this wraps Windows ConPTY.
type PtyHandle interface {
	io.ReadWriteCloser
	// Resize changes the PTY window size.
	Resize(cols, rows uint16) error
}
