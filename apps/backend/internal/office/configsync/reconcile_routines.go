package configsync

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

const kindRoutine = "routine"

// routineProjection is the owned-field view of a routine sync writes
// (AC-OFFICE-CONFIG-SYNC-003.5c: UpdateRoutineConfigFields owns description,
// task template, concurrency policy).
type routineProjection struct {
	Description       string
	TaskTemplate      string
	ConcurrencyPolicy models.RoutineConcurrencyPolicy
}

// buildFetchedRoutines parses every routines/*.yml file, resolves
// AC-OFFICE-CONFIG-SYNC-003.2/.3, and also returns the path of every file
// that failed to parse — AC-OFFICE-CONFIG-SYNC-003.12's deletion-sweep
// exemption input, resolved by the caller against the manifest.
func buildFetchedRoutines(files []fetchedFile) (
	fetched []fetchedEntity[routineProjection], warnings, unparsed []string,
) {
	type parsed struct {
		routine *parsedRoutine
		path    string
	}
	var ok []parsed
	for _, f := range files {
		pr, err := parseRoutineFile(f.path, f.content)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("routine file %q: %v; skipping", f.path, err))
			unparsed = append(unparsed, f.path)
			continue
		}
		if w := stemMismatchWarning(kindRoutine, f.path, pr.declaredName); w != "" {
			warnings = append(warnings, w)
		}
		ok = append(ok, parsed{routine: pr, path: f.path})
	}

	keyed := make([]keyedPath, len(ok))
	byPath := make(map[string]*parsedRoutine, len(ok))
	for i, p := range ok {
		keyed[i] = keyedPath{Key: p.routine.declaredName, Path: p.path}
		byPath[p.path] = p.routine
	}
	winners, collisionWarnings := resolveKeyCollisions(kindRoutine, keyed)
	warnings = append(warnings, collisionWarnings...)

	for key, winnerPath := range winners {
		pr := byPath[winnerPath]
		fetched = append(fetched, fetchedEntity[routineProjection]{
			Key:        key,
			SourcePath: winnerPath,
			Projection: routineProjection{
				Description:       pr.description,
				TaskTemplate:      pr.taskTemplate,
				ConcurrencyPolicy: models.RoutineConcurrencyPolicy(pr.concurrencyPolicy),
			},
		})
	}
	return fetched, warnings, unparsed
}

// routineOps adapts the office repository's routine CRUD to the generic
// six-case apply engine.
func routineOps(repo *sqlite.Repository) entityOps[routineProjection] {
	return entityOps[routineProjection]{
		kind: kindRoutine,
		list: func(ctx context.Context, workspaceID string) ([]entityRow[routineProjection], error) {
			routines, err := repo.ListRoutines(ctx, workspaceID)
			if err != nil {
				return nil, err
			}
			rows := make([]entityRow[routineProjection], len(routines))
			for i, r := range routines {
				rows[i] = entityRow[routineProjection]{
					ID:  r.ID,
					Key: r.Name,
					Projection: routineProjection{
						Description:       r.Description,
						TaskTemplate:      r.TaskTemplate,
						ConcurrencyPolicy: r.ConcurrencyPolicy,
					},
				}
			}
			return rows, nil
		},
		create: func(ctx context.Context, tx *sqlx.Tx, workspaceID, key, _ string, proj routineProjection) (string, error) {
			routine := &models.Routine{
				WorkspaceID:       workspaceID,
				Name:              key,
				Status:            "active",
				Description:       proj.Description,
				TaskTemplate:      proj.TaskTemplate,
				ConcurrencyPolicy: proj.ConcurrencyPolicy,
			}
			if err := repo.CreateRoutineTx(ctx, tx, routine); err != nil {
				return "", err
			}
			return routine.ID, nil
		},
		update: func(ctx context.Context, tx *sqlx.Tx, id string, proj routineProjection) error {
			return repo.UpdateRoutineConfigFieldsTx(ctx, tx, id, sqlite.RoutineConfigFields{
				Description:       proj.Description,
				TaskTemplate:      proj.TaskTemplate,
				ConcurrencyPolicy: proj.ConcurrencyPolicy,
			})
		},
		del: func(ctx context.Context, tx *sqlx.Tx, id string) error {
			return repo.DeleteRoutineTx(ctx, tx, id)
		},
	}
}
