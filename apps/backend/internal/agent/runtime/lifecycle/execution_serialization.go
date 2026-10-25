package lifecycle

import "context"

type serializedExecutionResult struct {
	value interface{}
	err   error
}

// doSerializedExecution shares session ownership with execution creation but
// never shares results. Cleanup and promotion must run their own identity checks;
// their nil success result cannot satisfy a concurrent execution lookup.
func (m *Manager) doSerializedExecution(
	ctx context.Context,
	key string,
	operation func(context.Context) (interface{}, error),
) (interface{}, error) {
	own := &serializedExecutionResult{}
	for {
		// A peer failure is not this operation's result either.
		value, _ := m.submitExecutionOperation(ctx, key, func(sharedCtx context.Context) (interface{}, error) {
			own.value, own.err = operation(sharedCtx)
			return own, nil
		})
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if result, ok := value.(*serializedExecutionResult); ok && result == own {
			return own.value, own.err
		}
	}
}
