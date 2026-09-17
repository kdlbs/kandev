package models

import "time"

type OperationRequest struct {
	OperationID            string `json:"operation_id,omitempty"`
	ExpectedIntentRevision *int64 `json:"expected_intent_revision,omitempty"`
}

type Operation struct {
	ID             string    `db:"id"`
	BindingID      string    `db:"binding_id"`
	OperationID    string    `db:"operation_id"`
	ConversationID string    `db:"conversation_id"`
	RunID          string    `db:"run_id"`
	Target         string    `db:"target"`
	RequestHash    string    `db:"request_hash"`
	IntentRevision int64     `db:"intent_revision"`
	BindingVersion int64     `db:"binding_version"`
	State          string    `db:"state"`
	ResponseJSON   string    `db:"response_json"`
	HTTPStatus     int       `db:"http_status"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}
