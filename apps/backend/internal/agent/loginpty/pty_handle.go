package loginpty

import "io"

// ptyHandle abstracts PTY operations across Unix and Windows.
// Unix wraps creack/pty (*os.File); Windows wraps ConPTY.
type ptyHandle interface {
	io.ReadWriteCloser
	Resize(cols, rows uint16) error
}
