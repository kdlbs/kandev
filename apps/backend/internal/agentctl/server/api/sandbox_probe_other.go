//go:build !darwin

package api

import "context"

func seatbeltHostProbe(context.Context) error { return nil }
