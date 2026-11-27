package agents

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOpenCodeNativeUnsupportedEvidenceSurvivesTransientFailure(t *testing.T) {
	fake := &fakeOpenCodeNative{now: time.Unix(1_000, 0), runs: []versionRun{
		{output: "opencode 3.0.0"},
		{err: errExitStatus1}, {err: errExitStatus1}, {err: errExitStatus1},
		{output: "opencode 2.0.18"},
		{err: errExitStatus1}, {err: errExitStatus1}, {err: errExitStatus1},
	}}
	d := newFakeOpenCodeDetector(fake, "/bin/opencode")
	_, _, rejected := d.detect(context.Background())
	if !IsUnsupportedOpenCodeMajorError(rejected) {
		t.Fatalf("initial rejection = %v, want unsupported major", rejected)
	}
	fake.now = fake.now.Add(11 * time.Minute)
	got, found, err := d.detect(context.Background())
	if !found || got != (OpenCodeNativeRuntime{}) || !IsUnsupportedOpenCodeMajorError(err) || !errors.Is(err, errExitStatus1) {
		t.Fatalf("after transient failure: runtime=%+v found=%v error=%v, want both definite rejection and probe failure", got, found, err)
	}
	got, found, err = d.detect(context.Background())
	if !found || err != nil || got.Version != "2.0.18" {
		t.Fatalf("fresh supported observation: runtime=%+v found=%v error=%v", got, found, err)
	}
	got, found, err = d.detect(context.Background())
	if !found || err != nil || got.Version != "2.0.18" {
		t.Fatalf("supported cache after recovery: runtime=%+v found=%v error=%v", got, found, err)
	}
}

func TestOpenCodeNativeCancellationRetainsObservedUnsupportedMajor(t *testing.T) {
	fake := &fakeOpenCodeNative{now: time.Unix(1_000, 0), runs: []versionRun{
		{output: "opencode 1.18.32"},
		{output: "opencode 3.0.0"},
		{err: errExitStatus1}, {err: errExitStatus1}, {err: errExitStatus1},
	}}
	d := newFakeOpenCodeDetector(fake, "/bin/opencode")
	if _, _, err := d.detect(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runVersion := d.runVersion
	d.runVersion = func(ctx context.Context, path string) ([]byte, error) {
		output, err := runVersion(ctx, path)
		cancel()
		return output, err
	}
	_, _, err := d.detect(ctx)
	if !errors.Is(err, context.Canceled) || !IsUnsupportedOpenCodeMajorError(err) {
		t.Fatalf("cancelled unsupported observation = %v, want both error identities", err)
	}
	d.runVersion = runVersion
	got, found, err := d.detect(context.Background())
	if !found || got != (OpenCodeNativeRuntime{}) || !IsUnsupportedOpenCodeMajorError(err) || !errors.Is(err, errExitStatus1) {
		t.Fatalf("later transient failure: runtime=%+v found=%v error=%v, want retained rejection", got, found, err)
	}
}
