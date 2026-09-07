package configsync

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

const kindProject = "project"

// projectProjection is the owned-field view of a project sync writes
// (AC-OFFICE-CONFIG-SYNC-003.5c: UpdateProjectConfigFields owns description,
// color, budget, repositories, executor config).
type projectProjection struct {
	Description    string
	Color          string
	BudgetCents    int
	Repositories   string
	ExecutorConfig string
}

// buildFetchedProjects parses every projects/*.yml file, resolves
// AC-OFFICE-CONFIG-SYNC-003.2/.3, and also returns the path of every file
// that failed to parse — AC-OFFICE-CONFIG-SYNC-003.12's deletion-sweep
// exemption input, resolved by the caller against the manifest.
func buildFetchedProjects(files []fetchedFile) (
	fetched []fetchedEntity[projectProjection], warnings, unparsed []string,
) {
	type parsed struct {
		project *parsedProject
		path    string
	}
	var ok []parsed
	for _, f := range files {
		pp, err := parseProjectFile(f.path, f.content)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("project file %q: %v; skipping", f.path, err))
			unparsed = append(unparsed, f.path)
			continue
		}
		if w := stemMismatchWarning(kindProject, f.path, pp.declaredName); w != "" {
			warnings = append(warnings, w)
		}
		ok = append(ok, parsed{project: pp, path: f.path})
	}

	keyed := make([]keyedPath, len(ok))
	byPath := make(map[string]*parsedProject, len(ok))
	for i, p := range ok {
		keyed[i] = keyedPath{Key: p.project.declaredName, Path: p.path}
		byPath[p.path] = p.project
	}
	winners, collisionWarnings := resolveKeyCollisions(kindProject, keyed)
	warnings = append(warnings, collisionWarnings...)

	for key, winnerPath := range winners {
		pp := byPath[winnerPath]
		fetched = append(fetched, fetchedEntity[projectProjection]{
			Key:        key,
			SourcePath: winnerPath,
			Projection: projectProjection{
				Description:    pp.description,
				Color:          pp.color,
				BudgetCents:    pp.budgetCents,
				Repositories:   pp.repositories,
				ExecutorConfig: pp.executorConfig,
			},
		})
	}
	return fetched, warnings, unparsed
}

// projectOps adapts the office repository's project CRUD to the generic
// six-case apply engine.
func projectOps(repo *sqlite.Repository) entityOps[projectProjection] {
	return entityOps[projectProjection]{
		kind: kindProject,
		list: func(ctx context.Context, workspaceID string) ([]entityRow[projectProjection], error) {
			projects, err := repo.ListProjects(ctx, workspaceID)
			if err != nil {
				return nil, err
			}
			rows := make([]entityRow[projectProjection], len(projects))
			for i, p := range projects {
				rows[i] = entityRow[projectProjection]{
					ID:  p.ID,
					Key: p.Name,
					Projection: projectProjection{
						Description:    p.Description,
						Color:          p.Color,
						BudgetCents:    p.BudgetCents,
						Repositories:   p.Repositories,
						ExecutorConfig: p.ExecutorConfig,
					},
				}
			}
			return rows, nil
		},
		create: func(ctx context.Context, tx *sqlx.Tx, workspaceID, key, _ string, proj projectProjection) (string, error) {
			project := &models.Project{
				WorkspaceID:    workspaceID,
				Name:           key,
				Status:         models.ProjectStatusActive,
				Description:    proj.Description,
				Color:          proj.Color,
				BudgetCents:    proj.BudgetCents,
				Repositories:   proj.Repositories,
				ExecutorConfig: proj.ExecutorConfig,
			}
			if err := repo.CreateProjectTx(ctx, tx, project); err != nil {
				return "", err
			}
			return project.ID, nil
		},
		update: func(ctx context.Context, tx *sqlx.Tx, id string, proj projectProjection) error {
			return repo.UpdateProjectConfigFieldsTx(ctx, tx, id, sqlite.ProjectConfigFields{
				Description:    proj.Description,
				Color:          proj.Color,
				BudgetCents:    proj.BudgetCents,
				Repositories:   proj.Repositories,
				ExecutorConfig: proj.ExecutorConfig,
			})
		},
		del: func(ctx context.Context, tx *sqlx.Tx, id string) error {
			return repo.DeleteProjectTx(ctx, tx, id)
		},
	}
}
