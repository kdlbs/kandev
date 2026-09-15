package github

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestRateCoordinatorAdmissionSerializesBackgroundPerPrincipalResource(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		label := outcomeMetricLabel("resource", string(ResourceCore), "reason", rateLimitBlockBackgroundBusy)
		before := readOutcomeCounter(t, githubBackgroundDeferralsTotal, label)
		coordinator := NewRateCoordinator(nil, nil)
		_, admission := coordinator.coordinate(defaultGitHubHost, AuthPrincipal{
			Kind: AuthPrincipalHuman, Login: "shared-user",
		}, nil)
		ctx := WithGitHubWorkClass(context.Background(), WorkClassBackground)
		firstRelease, err := admission.acquire(ctx, ResourceCore)
		if err != nil {
			t.Fatal(err)
		}

		secondAdmitted := make(chan struct{})
		go func() {
			release, acquireErr := admission.acquire(ctx, ResourceCore)
			if acquireErr == nil {
				release()
			}
			close(secondAdmitted)
		}()
		synctest.Wait()
		select {
		case <-secondAdmitted:
			t.Fatal("second background request entered an occupied principal/resource slot")
		default:
		}
		firstRelease()
		synctest.Wait()
		select {
		case <-secondAdmitted:
			t.Fatal("second background request was not paced after the prior request")
		default:
		}
		time.Sleep(time.Second)
		synctest.Wait()
		select {
		case <-secondAdmitted:
		default:
			t.Fatal("second background request remained blocked after the pacing interval")
		}
		if delta := readOutcomeCounter(t, githubBackgroundDeferralsTotal, label) - before; delta != 1 {
			t.Fatalf("background deferral counter delta = %d, want 1", delta)
		}
	})
}

func TestRateCoordinatorNonBlockingBackgroundAdmissionDefersWithoutHoldingWorker(t *testing.T) {
	coordinator := NewRateCoordinator(nil, nil)
	tracker, admission := coordinator.coordinate(defaultGitHubHost, AuthPrincipal{
		Kind: AuthPrincipalHuman, Login: "blocked-user",
	}, nil)
	retryAt := time.Now().Add(time.Hour)
	tracker.ObserveSecondary(ResourceCore, retryAt, RetrySourceConservativeFallback, "fixture")

	ctx := WithNonBlockingGitHubAdmission(
		WithGitHubWorkClass(context.Background(), WorkClassBackground),
	)
	release, err := admission.acquire(ctx, ResourceCore)
	if release != nil {
		t.Fatal("deferred admission returned a release function")
	}
	var deferred *AdmissionDeferredError
	if !errors.As(err, &deferred) {
		t.Fatalf("acquire error = %v, want AdmissionDeferredError", err)
	}
	if !deferred.RetryAt.Equal(retryAt) || deferred.RetrySource != RetrySourceConservativeFallback {
		t.Fatalf("deferred retry = (%s, %s), want (%s, %s)",
			deferred.RetryAt, deferred.RetrySource, retryAt, RetrySourceConservativeFallback)
	}

	tracker.ObserveSuccess(ResourceCore)
	if err := deferred.Wait(context.Background()); err != nil {
		t.Fatalf("deferred admission did not wake after tracker change: %v", err)
	}
}

func TestRateCoordinatorInteractiveAdmissionCancellationPreservesRateDetails(t *testing.T) {
	coordinator := NewRateCoordinator(nil, nil)
	tracker, admission := coordinator.coordinate(defaultGitHubHost, AuthPrincipal{
		Kind: AuthPrincipalHuman, Login: "interactive-user",
	}, nil)
	retryAt := time.Now().Add(time.Hour).UTC()
	tracker.ObserveSecondary(ResourceCore, retryAt, RetrySourceRetryAfter, "fixture")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := admission.acquire(ctx, ResourceCore)
	if err == nil {
		t.Fatal("interactive admission unexpectedly succeeded")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("admission error = %v, want context cancellation", err)
	}
	details, ok := OperationRateLimitFromError(err, time.Now().UTC())
	if !ok {
		t.Fatalf("operation rate details missing from canceled admission: %v", err)
	}
	if details.Kind != OperationRateLimitSecondaryThrottle || details.Resource != ResourceCore {
		t.Fatalf("operation rate details = %+v", details)
	}
	if details.RetryAt == nil || !details.RetryAt.Equal(retryAt) {
		t.Fatalf("retry_at = %v, want %s", details.RetryAt, retryAt)
	}
}

func TestRateCoordinatorNonBlockingBackgroundAdmissionWaitsForLocalPacing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		coordinator := NewRateCoordinator(nil, nil)
		_, admission := coordinator.coordinate(defaultGitHubHost, AuthPrincipal{
			Kind: AuthPrincipalHuman, Login: "paced-user",
		}, nil)
		ctx := WithNonBlockingGitHubAdmission(
			WithGitHubWorkClass(context.Background(), WorkClassBackground),
		)

		firstRelease, err := admission.acquire(ctx, ResourceCore)
		if err != nil {
			t.Fatalf("first acquire: %v", err)
		}
		firstRelease()

		secondResult := make(chan error, 1)
		go func() {
			release, acquireErr := admission.acquire(ctx, ResourceCore)
			if acquireErr == nil {
				release()
			}
			secondResult <- acquireErr
		}()
		synctest.Wait()
		select {
		case acquireErr := <-secondResult:
			t.Fatalf("second acquire returned before pacing elapsed: %v", acquireErr)
		default:
		}

		time.Sleep(defaultBackgroundPace)
		synctest.Wait()
		if acquireErr := <-secondResult; acquireErr != nil {
			t.Fatalf("second acquire: %v", acquireErr)
		}
	})
}

func TestRateCoordinatorNonBlockingBackgroundAdmissionDefersWhenThrottleStartsDuringLocalPacing(t *testing.T) {
	coordinator := NewRateCoordinator(nil, nil)
	tracker, admission := coordinator.coordinate(defaultGitHubHost, AuthPrincipal{
		Kind: AuthPrincipalHuman, Login: "paced-user",
	}, nil)
	ctx, cancel := context.WithCancel(WithNonBlockingGitHubAdmission(
		WithGitHubWorkClass(context.Background(), WorkClassBackground),
	))
	defer cancel()

	firstRelease, err := admission.acquire(ctx, ResourceCore)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	firstRelease()

	secondResult := make(chan error, 1)
	go func() {
		_, acquireErr := admission.acquire(ctx, ResourceCore)
		secondResult <- acquireErr
	}()
	select {
	case acquireErr := <-secondResult:
		t.Fatalf("second acquire returned before pacing elapsed: %v", acquireErr)
	case <-time.After(25 * time.Millisecond):
	}

	tracker.ObserveSecondary(
		ResourceCore, time.Now().Add(time.Hour), RetrySourceConservativeFallback, "fixture",
	)
	select {
	case acquireErr := <-secondResult:
		var deferred *AdmissionDeferredError
		if !errors.As(acquireErr, &deferred) {
			t.Fatalf("second acquire = %v, want AdmissionDeferredError", acquireErr)
		}
	case <-time.After(100 * time.Millisecond):
		cancel()
		<-secondResult
		t.Fatal("second acquire remained blocked after the throttle started")
	}
}

func TestRateAdmissionWaitForLocalPacingReturnsWhenDeadlineHasElapsed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := waitForLocalPacing(ctx, -time.Millisecond, make(chan struct{}), make(chan struct{})); err != nil {
		t.Fatalf("wait for elapsed pacing deadline: %v", err)
	}
}

func TestRateAdmissionWaitForLocalPacingUsesCapturedStateChange(t *testing.T) {
	stateChanged := make(chan struct{})
	close(stateChanged)
	if err := waitForLocalPacing(context.Background(), time.Hour, make(chan struct{}), stateChanged); err != nil {
		t.Fatalf("wait for captured state change: %v", err)
	}
}

func TestBackgroundDeferralReasonPrioritizesCurrentWaitersOverStalePacing(t *testing.T) {
	now := time.Now()
	decision := rateAdmissionDecision{backgroundReason: rateLimitBlockBackgroundPacing}
	if got := backgroundDeferralReason(decision, 1, false, now.Add(time.Second), now); got != rateLimitBlockInteractiveWaiting {
		t.Fatalf("interactive waiter reason = %q, want %q", got, rateLimitBlockInteractiveWaiting)
	}
	if got := backgroundDeferralReason(decision, 0, true, now.Add(time.Second), now); got != rateLimitBlockBackgroundBusy {
		t.Fatalf("busy request reason = %q, want %q", got, rateLimitBlockBackgroundBusy)
	}
}

func TestRateCoordinatorAdmissionGivesInteractiveWorkPriorityAfterRetryWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		coordinator := NewRateCoordinator(nil, nil)
		tracker, admission := coordinator.coordinate(defaultGitHubHost, AuthPrincipal{
			Kind: AuthPrincipalHuman, Login: "shared-user",
		}, nil)
		tracker.ObserveSecondary(
			ResourceCore, time.Now().Add(time.Hour), RetrySourceConservativeFallback, "fixture",
		)

		backgroundAdmitted := make(chan struct{})
		go func() {
			release, err := admission.acquire(
				WithGitHubWorkClass(context.Background(), WorkClassBackground), ResourceCore,
			)
			if err == nil {
				close(backgroundAdmitted)
				release()
			}
		}()
		interactiveRelease := make(chan func(), 1)
		go func() {
			release, err := admission.acquire(context.Background(), ResourceCore)
			if err == nil {
				interactiveRelease <- release
			}
		}()
		synctest.Wait()

		tracker.ObserveSuccess(ResourceCore)
		synctest.Wait()
		var releaseInteractive func()
		select {
		case releaseInteractive = <-interactiveRelease:
		default:
			t.Fatal("interactive request was not admitted when the retry window cleared")
		}
		select {
		case <-backgroundAdmitted:
			t.Fatal("background request was admitted while interactive work held priority")
		default:
		}

		releaseInteractive()
		synctest.Wait()
		select {
		case <-backgroundAdmitted:
		default:
			t.Fatal("background request did not resume after interactive work completed")
		}
	})
}

func TestRateCoordinatorPrincipalKeysShareOnlyTheSameUpstreamIdentity(t *testing.T) {
	coordinator := NewRateCoordinator(nil, nil)
	first, _ := coordinator.coordinate("github.com", AuthPrincipal{
		Kind: AuthPrincipalHuman, Login: "ALICE", WorkspaceID: "first",
	}, nil)
	same, _ := coordinator.coordinate("GITHUB.COM", AuthPrincipal{
		Kind: AuthPrincipalHuman, Login: "alice", WorkspaceID: "second",
	}, nil)
	different, _ := coordinator.coordinate("github.com", AuthPrincipal{
		Kind: AuthPrincipalHuman, Login: "bob", WorkspaceID: "first",
	}, nil)
	if first != same {
		t.Fatal("same human principal did not share rate state")
	}
	if first == different {
		t.Fatal("different human principals unexpectedly shared rate state")
	}

	app, _ := coordinator.coordinate("github.com", AuthPrincipal{
		Kind: AuthPrincipalApp, AppRegistrationID: "REG", InstallationID: 42,
		AppCredentialGeneration: 1, WorkspaceID: "first",
	}, nil)
	sameApp, _ := coordinator.coordinate("github.com", AuthPrincipal{
		Kind: AuthPrincipalApp, AppRegistrationID: "reg", InstallationID: 42,
		AppCredentialGeneration: 9, WorkspaceID: "second",
	}, nil)
	otherInstallation, _ := coordinator.coordinate("github.com", AuthPrincipal{
		Kind: AuthPrincipalApp, AppRegistrationID: "reg", InstallationID: 43,
	}, nil)
	if app != sameApp {
		t.Fatal("credential rotation split one GitHub App installation budget")
	}
	if app == otherInstallation {
		t.Fatal("different GitHub App installations unexpectedly shared rate state")
	}
}
