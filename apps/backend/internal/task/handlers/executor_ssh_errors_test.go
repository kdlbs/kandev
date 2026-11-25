package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/sshconnections"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestSSHExecutorWriteErrorsUseClientStatus(t *testing.T) {
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{sshconnections.ErrInvalidRoute, http.StatusBadRequest, ws.ErrorCodeValidation},
		{sshconnections.ErrForbidden, http.StatusForbidden, ws.ErrorCodeForbidden},
		{sshconnections.ErrConflict, http.StatusConflict, ws.ErrorCodeConflict},
		{sshconnections.ErrNotFound, http.StatusNotFound, ws.ErrorCodeNotFound},
		{sshconnections.ErrDisabled, http.StatusServiceUnavailable, ws.ErrorCodeUnavailable},
	} {
		status, code, message := executorResourceError(fmt.Errorf("save binding: %w", tt.err))
		if status != tt.status || code != tt.code || message != tt.err.Error() {
			t.Errorf("%v: got (%d, %q, %q)", tt.err, status, code, message)
		}
	}
}
