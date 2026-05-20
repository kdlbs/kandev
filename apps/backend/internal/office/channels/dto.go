package channels

import "github.com/kandev/kandev/internal/office/models"

// CreateChannelRequest is the request body for creating a channel.
type CreateChannelRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Platform    string `json:"platform"`
	Config      string `json:"config"`
	Status      string `json:"status"`
}

// ChannelListResponse wraps a list of channels.
type ChannelListResponse struct {
	Channels []*models.Channel `json:"channels"`
}
