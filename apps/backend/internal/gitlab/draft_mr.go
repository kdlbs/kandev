package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"go.uber.org/zap"
)

const detailedMergeStatusDraft = "draft_status"

var (
	// ErrMRIsDraft is returned by MergeMR when the MR is still a draft.
	ErrMRIsDraft = errors.New("gitlab: merge request is a draft")
	// ErrMRNotOpen is returned by MarkMRReady for an MR that is not open.
	ErrMRNotOpen = errors.New("gitlab: merge request is not open")
	// ErrDraftMarkerNotFound is returned when a draft MR has no leading draft marker to strip.
	ErrDraftMarkerNotFound = errors.New("gitlab: draft marker is not at the start of the title")
	// ErrDraftTitleEmpty is returned when stripping the draft marker would leave an empty title.
	ErrDraftTitleEmpty = errors.New("gitlab: title would be empty without the draft marker")
	// ErrMRReadAfterReady is returned when the title was written but the re-read failed or returned nothing.
	ErrMRReadAfterReady = errors.New("gitlab: merge request title updated but re-read failed")
	// ErrMRStillDraft is returned when the re-read after the title write still reports a draft.
	ErrMRStillDraft = errors.New("gitlab: merge request is still a draft after the title update")
	// ErrMRNilResult is returned when GitLab answers the first read without an MR.
	ErrMRNilResult = errors.New("gitlab: no merge request returned")
)

// MergeRefusedError marks an *APIError returned by the merge write itself with
// a status GitLab uses to refuse a merge. Error and Unwrap delegate to the
// wrapped error, so the message text is unchanged.
type MergeRefusedError struct {
	Status int
	Err    error
}

func (e *MergeRefusedError) Error() string { return e.Err.Error() }
func (e *MergeRefusedError) Unwrap() error { return e.Err }

func wrapMergeRefusal(err error) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.StatusCode {
	case http.StatusMethodNotAllowed, http.StatusNotAcceptable, http.StatusConflict, http.StatusUnprocessableEntity:
		return &MergeRefusedError{Status: apiErr.StatusCode, Err: err}
	}
	return err
}

// isDraftMR reports whether the MR is a draft: the draft flag or GitLab's
// draft merge status. A draft takes precedence over every other status.
func isDraftMR(draft bool, detailedMergeStatus string) bool {
	return draft || detailedMergeStatus == detailedMergeStatusDraft
}

var draftMarkers = []string{"draft:", "[draft]", "(draft)"}

// stripDraftMarkers removes every consecutive leading draft marker
// (case-insensitive "Draft:", "[Draft]", "(Draft)") together with the
// whitespace around it. found is false when the title has no leading marker.
func stripDraftMarkers(title string) (stripped string, found bool) {
	rest := strings.TrimSpace(title)
	for {
		matched := false
		for _, marker := range draftMarkers {
			if len(rest) >= len(marker) && strings.EqualFold(rest[:len(marker)], marker) {
				rest = strings.TrimLeftFunc(rest[len(marker):], unicode.IsSpace)
				matched, found = true, true
				break
			}
		}
		if !matched {
			break
		}
	}
	if !found {
		return strings.TrimSpace(title), false
	}
	return rest, true
}

// markMRReady runs the Mark as ready sequence against a client-specific read
// and title write. The write is skipped for an MR that is not an open draft.
func markMRReady(read func() (*MR, error), writeTitle func(title string) error) (*MR, error) {
	mr, err := read()
	if err != nil {
		return nil, err
	}
	if mr == nil {
		return nil, ErrMRNilResult
	}
	if normalizeMRState(mr.State) != mrStateOpen {
		return nil, ErrMRNotOpen
	}
	if !isDraftMR(mr.Draft, mr.DetailedMergeStatus) {
		return mr, nil
	}
	stripped, found := stripDraftMarkers(mr.Title)
	if !found {
		return nil, ErrDraftMarkerNotFound
	}
	if stripped == "" {
		return nil, ErrDraftTitleEmpty
	}
	if err := writeTitle(stripped); err != nil {
		return nil, err
	}
	reread, err := read()
	if err != nil || reread == nil {
		return nil, ErrMRReadAfterReady
	}
	if normalizeMRState(reread.State) != mrStateOpen {
		return reread, nil
	}
	if isDraftMR(reread.Draft, reread.DetailedMergeStatus) {
		return nil, ErrMRStillDraft
	}
	return reread, nil
}

// MarkMRReady strips the leading draft marker from the MR title and re-reads
// the MR. GitLab's REST API has no draft parameter.
func (c *PATClient) MarkMRReady(ctx context.Context, projectPath string, iid int) (*MR, error) {
	return markMRReady(
		func() (*MR, error) { return c.GetMR(ctx, projectPath, iid) },
		func(title string) error {
			body, err := json.Marshal(map[string]string{"title": title})
			if err != nil {
				return err
			}
			endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d", projectRef(projectPath), iid)
			if err := c.doWrite(ctx, http.MethodPut, endpoint, body, nil); err != nil {
				return fmt.Errorf("update MR !%d title: %w", iid, err)
			}
			return nil
		},
	)
}

// refreshCachedTaskMRs re-syncs the workspace's cached task MRs for one MR
// through the sync path, which honours a manual base-branch override. Rows
// without a recorded host are skipped. Failures are logged and skipped.
func (s *Service) refreshCachedTaskMRs(ctx context.Context, client Client, workspaceID, projectPath string, iid int) {
	byTask, err := s.ListTaskMRsByWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.Warn("list cached GitLab task MRs after mark ready", zap.Error(err))
		return
	}
	matches := matchingCachedTaskMRs(byTask, client.Host(), projectPath, iid)
	if len(matches) == 0 {
		return
	}

	status, err := client.GetMRStatus(ctx, projectPath, iid)
	if err != nil {
		s.logger.Warn("fetch GitLab MR status after mark ready", zap.Error(err))
		return
	}
	if status == nil || status.MR == nil {
		s.logger.Warn("GitLab returned no MR status after mark ready", zap.Int("iid", iid))
		return
	}
	s.refreshTaskMRsWithStatus(ctx, client, workspaceID, status, matches)
}

func matchingCachedTaskMRs(byTask map[string][]*TaskMR, host, projectPath string, iid int) []*TaskMR {
	var matches []*TaskMR
	for _, rows := range byTask {
		for _, row := range rows {
			if row.Host != "" && row.ProjectPath == projectPath && row.MRIID == iid &&
				sameConfiguredOrigin(row.Host, host) {
				matches = append(matches, row)
			}
		}
	}
	return matches
}

func (s *Service) refreshTaskMRsWithStatus(
	ctx context.Context, client Client, workspaceID string, status *MRStatus, matches []*TaskMR,
) {
	for _, existing := range matches {
		result, err := s.syncTaskMRWithStatus(
			ctx, client, existing.TaskID, existing.RepositoryID, existing.ProjectPath, existing.MRIID, status, existing,
		)
		if err != nil {
			s.logger.Warn("refresh cached GitLab task MR after mark ready",
				zap.String("task_id", existing.TaskID), zap.Error(err))
			continue
		}
		if result == nil || result.taskMR == nil {
			continue
		}
		s.publishTaskMRUpdated(ctx, workspaceID, result.taskMR)
	}
}

// MarkMRReadyForWorkspaceHost marks an MR ready through the workspace client
// and refreshes the cached task MRs once the title write has happened.
func (s *Service) MarkMRReadyForWorkspaceHost(ctx context.Context, workspaceID, expectedHost, projectPath string, iid int) (*MR, error) {
	var mr *MR
	err := s.RunWithWorkspaceClient(ctx, workspaceID, expectedHost, func(client Client) error {
		var actionErr error
		mr, actionErr = client.MarkMRReady(ctx, projectPath, iid)
		if actionErr == nil || errors.Is(actionErr, ErrMRStillDraft) || errors.Is(actionErr, ErrMRReadAfterReady) {
			s.refreshCachedTaskMRs(ctx, client, workspaceID, projectPath, iid)
		}
		return actionErr
	})
	return mr, err
}
