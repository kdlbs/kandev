package models

type WorkspaceContentQuery struct {
	SessionID string `form:"session_id"`
	MessageID string `form:"message_id"`
	Before    string `form:"before"`
	Source    string `form:"source"`
	Offset    int    `form:"offset"`
	Limit     int    `form:"limit"`
}
