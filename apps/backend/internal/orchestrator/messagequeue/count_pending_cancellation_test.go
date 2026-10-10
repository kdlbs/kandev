package messagequeue

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

type countPendingFailureRepository struct {
	Repository
	err  error
	read func(context.Context) error
}

func (r countPendingFailureRepository) CountPendingByTaskIDs(ctx context.Context, _ []string) (map[string]int, error) {
	if r.read != nil {
		return nil, r.read(ctx)
	}
	return nil, r.err
}

func TestCountPendingByTaskIDsSuppressesLogOnlyForCanceledContext(t *testing.T) {
	wrappedCancellation := fmt.Errorf("count pending: %w", context.Canceled)
	t.Run("canceled request returns original error without error log", func(t *testing.T) {
		core, observed := observer.New(zapcore.DebugLevel)
		log, err := logger.NewFromZap(zap.New(core))
		require.NoError(t, err)
		service := NewService(countPendingFailureRepository{err: wrappedCancellation}, 0, log)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = service.CountPendingByTaskIDs(ctx, []string{"task-1"})
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, wrappedCancellation.Error(), err.Error(), "the repository error chain remains intact")
		require.Zero(t, observed.FilterLevelExact(zapcore.ErrorLevel).Len())
	})

	t.Run("active request still logs the repository failure", func(t *testing.T) {
		core, observed := observer.New(zapcore.DebugLevel)
		log, err := logger.NewFromZap(zap.New(core))
		require.NoError(t, err)
		service := NewService(countPendingFailureRepository{err: wrappedCancellation}, 0, log)

		_, err = service.CountPendingByTaskIDs(context.Background(), []string{"task-1"})
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 1, observed.FilterLevelExact(zapcore.ErrorLevel).Len())
	})

	t.Run("expired deadline still logs a wrapped deadline failure", func(t *testing.T) {
		core, observed := observer.New(zapcore.DebugLevel)
		log, err := logger.NewFromZap(zap.New(core))
		require.NoError(t, err)
		service := NewService(countPendingFailureRepository{read: func(ctx context.Context) error {
			<-ctx.Done()
			return fmt.Errorf("count pending deadline: %w", ctx.Err())
		}}, 0, log)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Millisecond))
		defer cancel()

		_, err = service.CountPendingByTaskIDs(ctx, []string{"task-1"})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Equal(t, 1, observed.FilterLevelExact(zapcore.ErrorLevel).Len())
	})

	t.Run("unrelated repository failure concurrent with cancellation still logs", func(t *testing.T) {
		core, observed := observer.New(zapcore.DebugLevel)
		log, err := logger.NewFromZap(zap.New(core))
		require.NoError(t, err)
		databaseErr := errors.New("sqlite busy")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		service := NewService(countPendingFailureRepository{
			err: databaseErr,
			read: func(context.Context) error {
				cancel()
				return databaseErr
			},
		}, 0, log)

		_, err = service.CountPendingByTaskIDs(ctx, []string{"task-1"})
		require.ErrorIs(t, err, databaseErr)
		require.Equal(t, 1, observed.FilterLevelExact(zapcore.ErrorLevel).Len())
	})
}
