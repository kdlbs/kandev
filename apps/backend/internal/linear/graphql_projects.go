package linear

import (
	"context"
	"fmt"
)

const teamProjectsQuery = `
query TeamProjects($filter: ProjectFilter!, $first: Int!, $after: String) {
 projects(first: $first, after: $after, filter: $filter, orderBy: updatedAt) {
  nodes { id name }
  pageInfo { hasNextPage endCursor }
 }
}`

type projectsData struct {
	Projects struct {
		Nodes    []LinearProject `json:"nodes"`
		PageInfo *pageInfo       `json:"pageInfo"`
	} `json:"projects"`
}

// ListProjects returns all non-archived projects accessible to the team
// identified by teamKey.
func (c *GraphQLClient) ListProjects(ctx context.Context, teamKey string) ([]LinearProject, error) {
	if teamKey == "" {
		return nil, fmt.Errorf("teamKey required")
	}
	vars := map[string]interface{}{
		"filter": map[string]interface{}{
			"accessibleTeams": map[string]interface{}{
				"some": map[string]interface{}{"key": map[string]interface{}{"eq": teamKey}},
			},
		},
		"first": linearMaxPageSize,
	}
	return collectPages(ctx, "projects", vars, func(vars map[string]interface{}) ([]LinearProject, *pageInfo, error) {
		var data projectsData
		if err := c.do(ctx, teamProjectsQuery, vars, &data); err != nil {
			return nil, nil, err
		}
		return data.Projects.Nodes, data.Projects.PageInfo, nil
	})
}

const projectContentsQuery = `
query ProjectContents($filter: ProjectFilter!, $first: Int!) {
 projects(first: $first, filter: $filter, includeArchived: true) {
  nodes { id content }
 }
}`

type projectContentsData struct {
	Projects struct {
		Nodes []struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		} `json:"nodes"`
	} `json:"projects"`
}

// projectContentsBatchSize bounds one content lookup. Project content has no
// upper size limit, so a full page of nodes could exceed maxBodySize.
const projectContentsBatchSize = 25

// GetProjectContents returns the markdown content of each requested project,
// keyed by project ID. Projects that Linear does not return are absent from
// the map. IDs are requested in batches of projectContentsBatchSize.
func (c *GraphQLClient) GetProjectContents(ctx context.Context, projectIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(projectIDs))
	for start := 0; start < len(projectIDs); start += projectContentsBatchSize {
		batch := projectIDs[start:min(start+projectContentsBatchSize, len(projectIDs))]
		vars := map[string]interface{}{
			"filter": map[string]interface{}{"id": map[string]interface{}{"in": batch}},
			"first":  len(batch),
		}
		var data projectContentsData
		if err := c.do(ctx, projectContentsQuery, vars, &data); err != nil {
			return nil, err
		}
		for _, p := range data.Projects.Nodes {
			out[p.ID] = p.Content
		}
	}
	return out, nil
}
