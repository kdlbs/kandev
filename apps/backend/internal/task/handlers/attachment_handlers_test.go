package handlers

import (
	"errors"
	"mime/multipart"
	"net/http"
	"testing"
)

func TestIsAttachmentRequestTooLarge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "max bytes", err: &http.MaxBytesError{Limit: 10}, want: true},
		{name: "multipart form limit", err: multipart.ErrMessageTooLarge, want: true},
		{name: "other error", err: errors.New("multipart: malformed"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAttachmentRequestTooLarge(tt.err); got != tt.want {
				t.Fatalf("isAttachmentRequestTooLarge(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
