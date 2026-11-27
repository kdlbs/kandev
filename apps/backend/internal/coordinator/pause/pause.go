// Package pause holds the ports and the gate of the coordinator Pause state
// (docs/specs/coordinator/system-design/pause.md). The coordinator package
// implements the stored state and the Stopper; later packages (the dream tick)
// depend only on these ports, so no import cycle forms.
package pause

import (
	"context"
	"errors"
	"sync"

	"go.uber.org/zap"
)

// Gate answers whether a coordinator is paused. A caller decides on paused
// alone and never treats err as fatal: on a read error the answer is
// (true, err) where the fail-closed rule applies and (false, err) for the
// flag-off carve-out of a coordinator the process does not know to be paused.
type Gate interface {
	Active(ctx context.Context, coordinatorID string) (paused bool, err error)
}

// Stopper stops whatever a paused coordinator is running: its open unattended
// turns and its running dream. Stop is idempotent and returns joined errors
// for the log only.
type Stopper interface {
	Stop(ctx context.Context, coordinatorID string) error
}

// DreamStop is the registration point for the dream canceller (work order 04).
// It marks the coordinator's running dream failed with the reason paused,
// cancels the episode and cleans up leftover episodes. A nil DreamStop is a
// no-op.
type DreamStop func(ctx context.Context, coordinatorID string) error

// ReadFunc reads the stored paused state. A coordinator with no row reads as
// not paused.
type ReadFunc func(ctx context.Context, coordinatorID string) (bool, error)

// ErrEmptyID is the read error of an empty coordinator id.
var ErrEmptyID = errors.New("pause: empty coordinator id")

// KnownSet is the per-process set of coordinators known to be paused. It lets
// a flag-off process keep a paused coordinator paused when the state read
// fails, while every other coordinator behaves as phase 3 does.
type KnownSet struct {
	mu  sync.Mutex
	ids map[string]struct{}
}

// NewKnownSet returns an empty set.
func NewKnownSet() *KnownSet { return &KnownSet{ids: map[string]struct{}{}} }

// Has reports whether the coordinator is in the set.
func (k *KnownSet) Has(id string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	_, ok := k.ids[id]
	return ok
}

// Set adds or removes one coordinator.
func (k *KnownSet) Set(id string, paused bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if paused {
		k.ids[id] = struct{}{}
		return
	}
	delete(k.ids, id)
}

// Replace swaps the whole set for ids.
func (k *KnownSet) Replace(ids []string) {
	next := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		next[id] = struct{}{}
	}
	k.mu.Lock()
	k.ids = next
	k.mu.Unlock()
}

type gate struct {
	read      ReadFunc
	effective func() bool
	known     *KnownSet
	log       *zap.Logger
}

// NewGate builds the Gate. effective reports whether the phase 3.1 flag is
// effective; known is the flag-off carve-out set.
func NewGate(read ReadFunc, effective func() bool, known *KnownSet, log *zap.Logger) Gate {
	if log == nil {
		log = zap.NewNop()
	}
	return &gate{read: read, effective: effective, known: known, log: log}
}

func (g *gate) Active(ctx context.Context, coordinatorID string) (bool, error) {
	var paused bool
	var err error
	if coordinatorID == "" {
		err = ErrEmptyID
	} else {
		paused, err = g.read(ctx, coordinatorID)
	}
	if err == nil {
		return paused, nil
	}
	g.log.Warn("coordinator paused state unreadable",
		zap.String("coordinator_id", coordinatorID), zap.Error(err))
	if g.effective() || (g.known != nil && g.known.Has(coordinatorID)) {
		return true, err
	}
	return false, err
}
