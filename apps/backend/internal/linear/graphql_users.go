package linear

import (
	"context"
	"fmt"
)

const teamMembersQuery = `
query TeamMembers($filter: UserFilter!, $first: Int!, $after: String) {
 users(first: $first, after: $after, filter: $filter, orderBy: updatedAt) {
  nodes { id name displayName email avatarUrl }
  pageInfo { hasNextPage endCursor }
 }
}`

type usersData struct {
	Users struct {
		Nodes    []LinearUser `json:"nodes"`
		PageInfo *struct {
			HasNextPage *bool  `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"users"`
}

// ListUsers returns all members matching the team's membership scope.
func (c *GraphQLClient) ListUsers(ctx context.Context, teamKey string) ([]LinearUser, error) {
	if teamKey == "" {
		return nil, fmt.Errorf("teamKey required")
	}
	vars := map[string]interface{}{
		"filter": map[string]interface{}{
			"teamMemberships": map[string]interface{}{
				"some": map[string]interface{}{
					"team": map[string]interface{}{"key": map[string]interface{}{"eq": teamKey}},
				},
			},
		},
		"first": linearMaxPageSize,
	}
	out := make([]LinearUser, 0)
	cursors := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var data usersData
		if err := c.do(ctx, teamMembersQuery, vars, &data); err != nil {
			return nil, err
		}
		page := data.Users.PageInfo
		if page == nil || page.HasNextPage == nil {
			return nil, fmt.Errorf("invalid Linear users pagination: missing completion metadata")
		}
		out = append(out, data.Users.Nodes...)
		if !*page.HasNextPage {
			break
		}
		cursor := page.EndCursor
		if _, seen := cursors[cursor]; cursor == "" || seen {
			return nil, fmt.Errorf("invalid Linear users pagination: empty or repeated continuation cursor")
		}
		cursors[cursor] = struct{}{}
		vars["after"] = cursor
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
