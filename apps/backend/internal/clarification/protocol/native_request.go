package protocol

import (
	"errors"

	"github.com/google/uuid"
)

var ErrNativeQuestionCancelled = errors.New("provider cancelled the clarification request")

// NativeQuestionScope binds a clarification to one provider callback.
type NativeQuestionScope struct {
	SessionID       string `json:"session_id"`
	NativeRequestID string `json:"native_request_id,omitempty"`
}

func (s NativeQuestionScope) PendingID() string {
	if s.SessionID == "" || uuid.Validate(s.NativeRequestID) != nil {
		return ""
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("kandev/native-clarification/"+s.SessionID+"/"+s.NativeRequestID)).String()
}
