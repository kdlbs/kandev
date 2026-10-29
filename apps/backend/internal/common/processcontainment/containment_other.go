//go:build !linux

package processcontainment

import (
	"context"
	"os"
	"os/exec"
)

func Create() (*Identity, error)                          { return nil, ErrUnavailable }
func (i *Identity) Configure(*exec.Cmd) (*os.File, error) { return nil, ErrUnavailable }
func (i *Identity) Empty() (bool, error)                  { return false, ErrUnavailable }
func (i *Identity) Kill(context.Context) error            { return ErrUnavailable }
func (i *Identity) Remove() error                         { return ErrUnavailable }

func (i *Identity) Contains(int) (bool, error) { return false, ErrUnavailable }
