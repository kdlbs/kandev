package linear

import (
	"context"
	"fmt"
)

// pageInfo is Linear's connection completion metadata. HasNextPage is a
// pointer so an absent flag is distinguishable from a terminal page.
type pageInfo struct {
	HasNextPage *bool  `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// collectPages follows Linear's forward cursor pagination until the terminal
// page and returns every node in provider order. fetch performs one request
// with the given variables. Any failure, missing completion metadata, or an
// empty or repeated continuation cursor fails the whole operation without
// returning earlier pages.
func collectPages[T any](
	ctx context.Context,
	resource string,
	vars map[string]interface{},
	fetch func(vars map[string]interface{}) ([]T, *pageInfo, error),
) ([]T, error) {
	out := make([]T, 0)
	cursors := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		nodes, page, err := fetch(vars)
		if err != nil {
			return nil, err
		}
		if page == nil || page.HasNextPage == nil {
			return nil, fmt.Errorf("invalid Linear %s pagination: missing completion metadata", resource)
		}
		out = append(out, nodes...)
		if !*page.HasNextPage {
			break
		}
		cursor := page.EndCursor
		if _, seen := cursors[cursor]; cursor == "" || seen {
			return nil, fmt.Errorf("invalid Linear %s pagination: empty or repeated continuation cursor", resource)
		}
		cursors[cursor] = struct{}{}
		vars["after"] = cursor
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
