package journal

import bolt "go.etcd.io/bbolt"

// Callers hold j.mu through each transaction so Close and Compact cannot
// replace the database while an operation is in flight.
func (j *Journal) viewLocked(fn func(*bolt.Tx) error) error {
	if j.db == nil {
		return bolt.ErrDatabaseNotOpen
	}
	return j.db.View(fn)
}

func (j *Journal) updateLocked(fn func(*bolt.Tx) error) error {
	if j.db == nil {
		return bolt.ErrDatabaseNotOpen
	}
	return j.db.Update(fn)
}
