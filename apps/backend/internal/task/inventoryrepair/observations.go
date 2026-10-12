package inventoryrepair

import (
	"context"
	"errors"
)

type observation struct {
	Query  string `json:"query"`
	Args   []any  `json:"args"`
	Before string `json:"before"`
	After  string `json:"after,omitempty"`
}

func (j *journal) checkObservations(ctx context.Context, db queryer, after bool) error {
	for _, o := range j.Observations {
		rows, err := queryRows(ctx, db, o.Query, o.Args...)
		if err != nil {
			return err
		}
		want := o.Before
		if after {
			want = o.After
		}
		if want == "" || digest(rows) != want {
			return errors.New("task, session, environment, repository, or cleanup evidence changed since repair observation")
		}
	}
	return nil
}

func (j *journal) capturePublishedObservations(ctx context.Context, db queryer) error {
	for i, o := range j.Observations {
		rows, err := queryRows(ctx, db, o.Query, o.Args...)
		if err != nil {
			return err
		}
		j.Observations[i].After = digest(rows)
	}
	return j.save()
}
