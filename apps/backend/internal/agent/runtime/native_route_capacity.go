package runtime

import (
	"context"
	"errors"
	"strings"
)

type nativeRouteAdmission struct {
	admit   func(context.Context, string, string) error
	release func(string)
}

type nativeAdmissionWaiter interface{ AwaitNativeRoute(context.Context) error }

// SetNativeRouteAdmission installs the existing capacity owner at bootstrap.
// No admission key or permit is carried in an inference identity.
func (r *ProfileExecutionResolver) SetNativeRouteAdmission(admit func(context.Context, string, string) error, release func(string)) {
	r.nativeAdmission = &nativeRouteAdmission{admit: admit, release: release}
}

func nativeInferenceRoute(id string) bool {
	return strings.HasPrefix(id, "native:utility-call:") || strings.HasPrefix(id, "native:review-run:")
}

func (r *ProfileExecutionResolver) admitNativeRoute(ctx context.Context, id, profileID string) error {
	if r.nativeAdmission == nil || r.nativeAdmission.admit == nil || !r.NativeRouteOwnersEnabled() || !nativeInferenceRoute(id) {
		return nil
	}
	err := r.nativeAdmission.admit(ctx, id, profileID)
	var wait nativeAdmissionWaiter
	if strings.HasPrefix(id, "native:utility-call:") && errors.As(err, &wait) {
		return wait.AwaitNativeRoute(ctx)
	}
	return err
}

// ReleaseNativeRoute removes only a pre-selection reservation. A started or
// uncertain child remains in the existing retained inference binding store.
func (r *ProfileExecutionResolver) ReleaseNativeRoute(id string) {
	if r == nil || r.nativeAdmission == nil || r.nativeAdmission.release == nil || !nativeInferenceRoute(id) {
		return
	}
	r.nativeAdmission.release(id)
}

func (r *ProfileExecutionResolver) nativeStaticRouteID(id string) string {
	if r.NativeRouteOwnersEnabled() && nativeInferenceRoute(id) {
		return id
	}
	return ""
}
