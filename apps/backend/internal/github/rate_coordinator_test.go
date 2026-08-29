package github

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestRateCoordinatorAdmissionSerializesBackgroundPerPrincipalResource(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
	})
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
