package process

import (
	"github.com/google/uuid"
)

// UUIDv7 orders receipts so expired retries can be rejected without retaining
// unbounded tombstones. In-flight receipts are never evicted.
func (o *PassthroughOwner) makeInputRoomLocked(id string) error {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.Version() != 7 || parsed.String() != id || id <= o.inputFloor {
		return ErrPassthroughConflict
	}
	if len(o.inputs) < 4096 {
		return nil
	}
	var oldest string
	prefix := o.current.ProcessID + ":"
	for key, receipt := range o.inputs {
		select {
		case <-receipt.done:
			candidate := key[len(prefix):]
			if oldest == "" || candidate < oldest {
				oldest = candidate
			}
		default:
		}
	}
	if oldest == "" || id <= oldest {
		return ErrPassthroughConflict
	}
	delete(o.inputs, prefix+oldest)
	if oldest > o.inputFloor {
		o.inputFloor = oldest
	}
	return nil
}
