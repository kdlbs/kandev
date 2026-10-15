package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

func executeAwaitFile(e *emitter, line string) {
	ctx := e.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	path := extractStringArg(line, "e2e:await_file(")
	if err := waitForScriptFile(ctx, path); err != nil && ctx.Err() == nil {
		e.text(fmt.Sprintf("Script error: await_file: %v", err))
	}
}

func waitForScriptFile(ctx context.Context, path string) error {
	if path == "" {
		return errors.New("release path is required")
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err == nil {
			if !info.Mode().IsRegular() {
				return errors.New("release marker must be a regular file")
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
