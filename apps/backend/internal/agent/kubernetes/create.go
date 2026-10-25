package kubernetes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/url"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const CreateNonceAnnotation = "kandev.ai/create-nonce"

// StampCreateNonce binds ambiguous-create reconciliation to this exact create
// attempt. The annotation is Kandev-owned, so the admission validators require
// the API result to preserve it exactly.
func StampCreateNonce(object metav1.Object) error {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return err
	}
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	annotations[CreateNonceAnnotation] = hex.EncodeToString(bytes)
	object.SetAnnotations(annotations)
	return nil
}

// IsAmbiguousCreateError reports whether a create call may have reached the API
// server before its result was lost. Definite admission/API rejections must
// never authorize adopting an exact-name object discovered after the error.
func IsAmbiguousCreateError(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var statusErr apierrors.APIStatus
	if errors.As(err, &statusErr) {
		code := statusErr.Status().Code
		return code >= 500 && code <= 599
	}
	var transportErr *url.Error
	if errors.As(err, &transportErr) {
		return true
	}
	var networkErr net.Error
	return errors.As(err, &networkErr)
}
