package kubernetes

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsAmbiguousCreateError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "transport reset", err: &net.OpError{Op: "write", Net: "tcp", Err: syscall.ECONNRESET}, want: true},
		{name: "unexpected EOF", err: io.ErrUnexpectedEOF, want: true},
		{name: "caller deadline", err: context.DeadlineExceeded, want: true},
		{name: "server timeout", err: apierrors.NewServerTimeout(schema.GroupResource{Resource: "pods"}, "create", 1), want: true},
		{name: "internal server error", err: apierrors.NewInternalError(errors.New("apiserver unavailable")), want: true},
		{name: "already exists", err: apierrors.NewAlreadyExists(schema.GroupResource{Resource: "pods"}, "agent")},
		{name: "conflict", err: apierrors.NewConflict(schema.GroupResource{Resource: "pods"}, "agent", errors.New("conflict"))},
		{name: "too many requests", err: apierrors.NewTooManyRequests("rate limited", 1)},
		{name: "forbidden", err: apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "agent", errors.New("denied"))},
		{name: "invalid", err: apierrors.NewInvalid(schema.GroupKind{Group: "", Kind: "Pod"}, "agent", nil)},
		{name: "unsupported media type", err: &apierrors.StatusError{ErrStatus: metav1.Status{
			Code: http.StatusUnsupportedMediaType, Reason: metav1.StatusReasonUnsupportedMediaType,
		}}},
		{name: "ordinary error", err: errors.New("definite local failure")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, IsAmbiguousCreateError(test.err))
		})
	}
}
