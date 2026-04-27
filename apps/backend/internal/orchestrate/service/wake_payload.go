package service

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/orchestrate/repository/sqlite"
)

// WakePayload is the pre-computed task context injected into the
// KANDEV_WAKE_PAYLOAD_JSON environment variable for agent sessions.
type WakePayload struct {
	Task          *WakePayloadTask     `json:"task,omitempty"`
	NewComments   []WakePayloadComment `json:"newComments,omitempty"`
	CommentWindow *CommentWindow       `json:"commentWindow,omitempty"`
}

// WakePayloadTask contains the essential task fields for agent context.
type WakePayloadTask struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
	Priority    int    `json:"priority,omitempty"`
	Project     string `json:"project,omitempty"`
}

// WakePayloadComment contains a recent task comment.
type WakePayloadComment struct {
	Author     string `json:"author"`
	AuthorType string `json:"authorType"`
	Body       string `json:"body"`
	CreatedAt  string `json:"createdAt"`
}

// CommentWindow indicates how many total comments exist and whether
// the agent should fetch more via the API.
type CommentWindow struct {
	Total     int  `json:"total"`
	Included  int  `json:"included"`
	FetchMore bool `json:"fetchMore"`
}

// wakePayloadCommentLimit is the maximum number of recent comments
// included in the wake payload.
const wakePayloadCommentLimit = 5

// BuildWakePayload constructs the JSON wake payload for a wakeup request.
// Returns an empty string when no task_id is present in the wakeup payload.
func (s *Service) BuildWakePayload(ctx context.Context, wakeup *WakeupPayloadInput) (string, error) {
	taskID := extractField(wakeup.Payload, "task_id")
	if taskID == "" {
		return "", nil
	}

	task, err := s.repo.GetTaskBasicInfo(ctx, taskID)
	if err != nil {
		return "", err
	}
	if task == nil {
		return "", nil
	}

	projectName := s.resolveProjectName(ctx, task.ProjectID)

	comments, err := s.repo.ListRecentTaskComments(ctx, taskID, wakePayloadCommentLimit)
	if err != nil {
		return "", err
	}
	total, err := s.repo.CountTaskComments(ctx, taskID)
	if err != nil {
		return "", err
	}

	payload := WakePayload{
		Task:        mapTaskToPayload(task, projectName),
		NewComments: mapCommentsToPayload(comments),
		CommentWindow: &CommentWindow{
			Total:     total,
			Included:  len(comments),
			FetchMore: total > len(comments),
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WakeupPayloadInput is a lightweight struct to pass wakeup data for
// payload building without requiring the full models.WakeupRequest.
type WakeupPayloadInput struct {
	Payload string
}

func mapTaskToPayload(task *sqlite.TaskBasicInfo, projectName string) *WakePayloadTask {
	return &WakePayloadTask{
		ID:          task.Identifier, // use identifier for display
		Identifier:  task.Identifier,
		Title:       task.Title,
		Description: task.Description,
		Priority:    task.Priority,
		Project:     projectName,
	}
}

func mapCommentsToPayload(comments []*sqlite.RecentComment) []WakePayloadComment {
	out := make([]WakePayloadComment, 0, len(comments))
	for _, c := range comments {
		out = append(out, WakePayloadComment{
			Author:     c.AuthorID,
			AuthorType: c.AuthorType,
			Body:       c.Body,
			CreatedAt:  c.CreatedAt,
		})
	}
	return out
}

// resolveProjectName looks up the project name for a project ID. Returns
// empty string on error or if the project is not found.
func (s *Service) resolveProjectName(ctx context.Context, projectID string) string {
	if projectID == "" {
		return ""
	}
	project, err := s.GetProjectFromConfig(ctx, projectID)
	if err != nil || project == nil {
		return ""
	}
	return project.Name
}
