package routingerr

import (
	"context"
	"testing"
)

type fakeProber struct {
	calls int
	out   *Error
}

func (f *fakeProber) Probe(_ context.Context, _ ProbeInput) *Error {
	f.calls++
	return f.out
}

func TestProberRegistry_RegisterAndGet(t *testing.T) {
	p := &fakeProber{}
	RegisterProber("test-provider", p)
	got, ok := GetProber("test-provider")
	if !ok {
		t.Fatal("expected prober to be registered")
	}
	if e := got.Probe(context.Background(), ProbeInput{ProviderID: "test-provider"}); e != nil {
		t.Fatal("expected nil error from fake")
	}
	if p.calls != 1 {
		t.Fatalf("expected 1 call, got %d", p.calls)
	}
}

func TestProberRegistry_NotFound(t *testing.T) {
	if _, ok := GetProber("nonexistent-provider-xyz"); ok {
		t.Fatal("expected miss for unknown provider")
	}
}

func TestProberRegistry_LastWriteWins(t *testing.T) {
	first := &fakeProber{}
	second := &fakeProber{}
	RegisterProber("dup-provider", first)
	RegisterProber("dup-provider", second)
	got, _ := GetProber("dup-provider")
	_ = got.Probe(context.Background(), ProbeInput{ProviderID: "dup-provider"})
	if first.calls != 0 || second.calls != 1 {
		t.Fatalf("expected second prober to receive call, got first=%d second=%d", first.calls, second.calls)
	}
}
