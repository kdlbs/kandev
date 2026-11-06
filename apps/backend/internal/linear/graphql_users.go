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
		PageInfo *pageInfo    `json:"pageInfo"`
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
	return collectPages(ctx, "users", vars, func(vars map[string]interface{}) ([]LinearUser, *pageInfo, error) {
		var data usersData
		if err := c.do(ctx, teamMembersQuery, vars, &data); err != nil {
			return nil, nil, err
		}
		return data.Users.Nodes, data.Users.PageInfo, nil
	})
}
