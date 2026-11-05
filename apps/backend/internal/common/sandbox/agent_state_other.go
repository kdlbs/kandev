//go:build !linux

package sandbox

import "os"

func lockAgentState(string) (*os.File, error) { return nil, ErrInvalidPolicy }
