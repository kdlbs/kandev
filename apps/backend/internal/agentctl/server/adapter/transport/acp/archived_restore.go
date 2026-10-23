package acp

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/coder/acp-go-sdk"
)

const archiveWindowsOS = "windows"

func airSessionMetadata(meta map[string]any) map[string]any {
	jetbrains, _ := meta["jetbrains"].(map[string]any)
	air, _ := jetbrains["air"].(map[string]any)
	return air
}

func codexArchiveCapability(meta map[string]any) bool {
	air := airSessionMetadata(meta)
	capabilities, _ := air["capabilities"].([]any)
	for _, capability := range capabilities {
		if capability == "sessionArchive" {
			return true
		}
	}
	return false
}

func (a *Adapter) restoreSessionState(ctx context.Context, conn *acp.ClientSideConnection, capabilities acp.AgentCapabilities, req acp.LoadSessionRequest) (acp.LoadSessionResponse, error) {
	response, err := a.restoreSessionStateOnce(ctx, conn, capabilities, req)
	if !archiveLookupEligible(err) {
		return response, err
	}
	a.mu.RLock()
	supported := a.archivedSessionRestoreSupported
	a.mu.RUnlock()
	if a.agentID != codexAgentID || a.cfg.Extra["allow_archived_thread_restore"] != "true" || !supported || capabilities.SessionCapabilities.List == nil {
		return response, err
	}
	archived, lookupErr := findArchivedACPSession(ctx, conn, req)
	if lookupErr != nil {
		return response, err
	}
	if !archived {
		return response, err
	}
	if _, restoreErr := conn.CallExtension(ctx, "_session/unarchive", map[string]any{"sessionId": req.SessionId}); restoreErr != nil {
		return response, restoreErr
	}
	return a.restoreSessionStateOnce(ctx, conn, capabilities, req)
}

func archiveLookupEligible(err error) bool {
	var request *acp.RequestError
	if !errors.As(err, &request) {
		return false
	}
	// Internal-error responses can represent an archived thread. Only the
	// provider's exact archived listing, never the error text, authorizes it.
	return request.Code == -32002 || request.Code == -32603
}

func findArchivedACPSession(ctx context.Context, conn *acp.ClientSideConnection, req acp.LoadSessionRequest) (bool, error) {
	for _, cwd := range archiveDirectoryQueries(req.Cwd) {
		found, err := findArchivedACPDirectory(ctx, conn, req.SessionId, cwd)
		if err != nil || found {
			return found, err
		}
	}
	return false, nil
}

func findArchivedACPDirectory(ctx context.Context, conn *acp.ClientSideConnection, sessionID acp.SessionId, cwd string) (bool, error) {
	query := acp.ListSessionsRequest{Cwd: &cwd, Meta: map[string]any{
		"jetbrains": map[string]any{"air": map[string]any{"list": map[string]any{
			"archived": "archived", "includeWorktrees": false, "limit": 100,
		}}},
	}}
	for ctx.Err() == nil {
		page, err := conn.ListSessions(ctx, query)
		if err != nil {
			return false, err
		}
		for _, session := range page.Sessions {
			if session.SessionId == sessionID && sameArchiveWorkingDirectory(session.Cwd, cwd) {
				archived, _ := airSessionMetadata(session.Meta)["archived"].(bool)
				return archived, nil
			}
		}
		if page.NextCursor == nil {
			return false, nil
		}
		if query.Cursor != nil && *page.NextCursor == *query.Cursor {
			return false, errors.New("native archive listing did not advance")
		}
		query.Cursor = page.NextCursor
	}
	return false, ctx.Err()
}

func archiveDirectoryQueries(cwd string) []string {
	queries := []string{cwd}
	if runtime.GOOS != archiveWindowsOS {
		return queries
	}
	plain := archiveWorkingDirectory(cwd)
	alias := plain
	if strings.HasPrefix(plain, `\\`) {
		alias = `\\?\UNC\` + strings.TrimPrefix(plain, `\\`)
	} else if filepath.IsAbs(plain) {
		alias = `\\?\` + plain
	}
	if alias == cwd {
		alias = plain
	}
	if alias != cwd {
		queries = append(queries, alias)
	}
	return queries
}

func sameArchiveWorkingDirectory(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	left, right = archiveWorkingDirectory(left), archiveWorkingDirectory(right)
	if runtime.GOOS == archiveWindowsOS {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func archiveWorkingDirectory(value string) string {
	if runtime.GOOS == archiveWindowsOS {
		value = strings.ReplaceAll(value, "/", "\\")
		if len(value) >= 8 && strings.EqualFold(value[:8], `\\?\UNC\`) {
			value = `\\` + value[8:]
		} else {
			value = strings.TrimPrefix(value, `\\?\`)
		}
	}
	return filepath.Clean(value)
}
