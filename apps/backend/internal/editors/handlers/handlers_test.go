package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/editors/service"
)

func TestServiceErrorStatus(t *testing.T) {
	tests := []struct {
		err      error
		expected int
	}{
		{service.ErrWorkspaceNotFound, http.StatusNotFound},
		{service.ErrEditorNotFound, http.StatusNotFound},
		{service.ErrEditorConfigInvalid, http.StatusBadRequest},
		{service.ErrEditorUnavailable, http.StatusConflict},
		{fmt.Errorf("wrapped: %w", service.ErrWorkspaceNotFound), http.StatusNotFound},
		{errors.New("something else"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			if got := serviceErrorStatus(tt.err); got != tt.expected {
				t.Errorf("serviceErrorStatus(%v) = %d, want %d", tt.err, got, tt.expected)
			}
		})
	}
}
