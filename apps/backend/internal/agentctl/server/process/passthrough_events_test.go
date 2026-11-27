package process

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPassthroughReplayPagesFitReaderAndPreserveAllEvents(t *testing.T) {
	o := receiptTestOwner(t)
	o.manager.cfg.SessionID, o.manager.cfg.InstanceID = uuid.NewString(), uuid.NewString()
	o.limit = 4096
	o.current.Status = types.ProcessStatusRunning
	o.current.PassthroughTarget = types.PassthroughTarget{ProcessID: uuid.NewString(), Generation: 1}
	for range o.limit {
		o.emitLocked("submitted", nil)
	}
	var after uint64
	for after < o.current.Sequence {
		page, err := o.Events(context.Background(), after, false)
		require.NoError(t, err)
		require.False(t, page.Gap)
		require.NotEmpty(t, page.Events)
		data, err := json.Marshal(page)
		require.NoError(t, err)
		require.LessOrEqual(t, len(data), 1024*1024, "every replay page must fit the backend reader")
		for _, event := range page.Events {
			after++
			require.Equal(t, after, event.Sequence)
		}
		require.Equal(t, after, page.Sequence, "the stream cursor must not skip events in later pages")
	}
	require.Equal(t, uint64(4096), after)
}
