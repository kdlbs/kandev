package executor

import "context"

type nativeRestoreStartupContextKey struct{}

func isNativeRestoreStartupContext(ctx context.Context) bool {
	owned, _ := ctx.Value(nativeRestoreStartupContextKey{}).(bool)
	return owned
}

func awaitNativeRestoreStartup(ctx context.Context, result <-chan error) error {
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
