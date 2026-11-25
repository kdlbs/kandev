package plugins

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestFilePreviewGrantRequiresExactOwnerApproval(t *testing.T) {
	grants := newFilePreviewGrants()
	defer grants.close()
	reads := 0
	source := &filePreviewSource{Read: func(ctx context.Context, w io.Writer) error { reads++; _, err := w.Write([]byte("mesh")); return err }, Close: func() {}}
	scope := filePreviewScope{UserID: "alice", WorkspaceID: "w", InstallationID: "i", Digest: "d"}
	token, err := grants.add(scope, 1, true, source)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := grants.read(t.Context(), token, scope, 1, false, &out); err == nil || reads != 0 {
		t.Fatal("read before approval")
	}
	foreign := scope
	foreign.UserID = "bob"
	if err := grants.read(t.Context(), token, foreign, 1, true, &out); err == nil || reads != 0 {
		t.Fatal("foreign approval")
	}
	if err := grants.read(t.Context(), token, scope, 1, true, &out); err != nil || out.String() != "mesh" {
		t.Fatalf("approved read %v %q", err, out.String())
	}
	if err := grants.read(t.Context(), token, scope, 1, true, &out); err == nil || reads != 1 {
		t.Fatal("replayed approval")
	}
}

func TestFilePreviewGrantRejectsChangedPolicy(t *testing.T) {
	grants := newFilePreviewGrants()
	defer grants.close()
	reads := 0
	closed := false
	source := &filePreviewSource{Read: func(context.Context, io.Writer) error { reads++; return nil }, Close: func() { closed = true }}
	scope := filePreviewScope{UserID: "u", WorkspaceID: "w", InstallationID: "i", Digest: "d"}
	token, err := grants.add(scope, 1, true, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := grants.read(t.Context(), token, scope, 2, true, &bytes.Buffer{}); err == nil || reads != 0 {
		t.Fatal("stale policy read")
	}
	grants.close()
	if !closed {
		t.Fatal("source handle leaked")
	}
}

func TestFilePreviewGrantRevokesActiveRead(t *testing.T) {
	g := newFilePreviewGrants()
	defer g.close()
	scope := filePreviewScope{UserID: "user", WorkspaceID: "workspace", InstallationID: "install", Digest: "digest"}
	started := make(chan struct{})
	done := make(chan error, 1)
	token, err := g.add(scope, 0, true, &filePreviewSource{Read: func(ctx context.Context, _ io.Writer) error { close(started); <-ctx.Done(); return ctx.Err() }, Close: func() {}})
	if err != nil {
		t.Fatal(err)
	}
	go func() { done <- g.read(t.Context(), token, scope, 0, true, io.Discard) }()
	<-started
	g.revokeScope(scope)
	if err := <-done; err == nil {
		t.Fatal("read survived revocation")
	}
}

func TestFilePreviewGrantOneTransferPerUser(t *testing.T) {
	g := newFilePreviewGrants()
	defer g.close()
	scope := filePreviewScope{UserID: "user", WorkspaceID: "workspace", InstallationID: "install", Digest: "digest"}
	started := make(chan struct{})
	finish := make(chan struct{})
	done := make(chan error, 1)
	source := &filePreviewSource{Read: func(context.Context, io.Writer) error { close(started); <-finish; return nil }, Close: func() {}}
	first, _ := g.add(scope, 0, false, source)
	secondReads := 0
	second, _ := g.add(scope, 0, false, &filePreviewSource{Read: func(context.Context, io.Writer) error { secondReads++; return nil }, Close: func() {}})
	go func() { done <- g.read(t.Context(), first, scope, 0, false, io.Discard) }()
	<-started
	err := g.read(t.Context(), second, scope, 0, false, io.Discard)
	close(finish)
	<-done
	if err == nil || secondReads != 0 {
		t.Fatal("concurrent transfers exceeded user budget")
	}
}

func TestPluginStatusChangeRevokesFilePreviewGrant(t *testing.T) {
	svc, _, _ := newTestService(t)
	record := installTestPlugin(t, svc, "kandev-plugin-preview")
	grants := svc.filePreviewGrantStore()
	scope := filePreviewScope{
		UserID:         "user",
		WorkspaceID:    "workspace",
		InstallationID: record.InstallationID,
		Digest:         "digest",
	}
	closed := false
	token, err := grants.add(scope, 1, false, &filePreviewSource{
		Read:  func(context.Context, io.Writer) error { return nil },
		Close: func() { closed = true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Disable(record.ID); err != nil {
		t.Fatalf("Disable plugin: %v", err)
	}
	if err := grants.read(t.Context(), token, scope, 1, false, io.Discard); err == nil {
		t.Fatal("preview grant remained usable after plugin disable")
	}
	if !closed {
		t.Fatal("preview source remained open after plugin disable")
	}
}
