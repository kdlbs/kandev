package waveidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

func TestWaveString_JoinsParentAndMembersWithPipeAndComma(t *testing.T) {
	got := WaveString("parent-1", []string{"child-1", "child-2"})
	want := "parent-1|child-1,child-2"
	if got != want {
		t.Fatalf("WaveString() = %q, want %q", got, want)
	}
}

func TestWaveString_EmptyMembers(t *testing.T) {
	got := WaveString("parent-1", nil)
	want := "parent-1|"
	if got != want {
		t.Fatalf("WaveString() = %q, want %q", got, want)
	}
}

func TestWaveString_DeterministicForFixedInput(t *testing.T) {
	ids := []string{"a", "b", "c"}
	first := WaveString("p", ids)
	second := WaveString("p", ids)
	if first != second {
		t.Fatalf("WaveString() not deterministic: %q != %q", first, second)
	}
}

func TestWaveString_DifferentMemberSetsDiffer(t *testing.T) {
	a := WaveString("p", []string{"1", "2"})
	b := WaveString("p", []string{"1", "3"})
	if a == b {
		t.Fatalf("expected different member sets to produce different wave strings, got %q for both", a)
	}
}

func TestWaveString_DifferentParentsWithSameMembersDiffer(t *testing.T) {
	a := WaveString("parent-a", []string{"1", "2"})
	b := WaveString("parent-b", []string{"1", "2"})
	if a == b {
		t.Fatalf("expected different parents to produce different wave strings, got %q for both", a)
	}
}

func TestWaveKey_IsPureFunctionOfWaveString(t *testing.T) {
	ids := []string{"child-1", "child-2"}
	key1 := WaveKey("parent-1", ids)
	key2 := WaveKey("parent-1", ids)
	if key1 != key2 {
		t.Fatalf("WaveKey() not deterministic: %q != %q", key1, key2)
	}

	// Equal wave strings must always yield equal keys (AC-...-001.11): the
	// key is exactly the documented digest of the wave string, recomputed
	// here independently of the production implementation.
	s := WaveString("parent-1", ids)
	sum := sha256.Sum256([]byte(s))
	want := fmt.Sprintf("task_children_completed:parent-1:%s", hex.EncodeToString(sum[:]))
	if key1 != want {
		t.Fatalf("WaveKey() = %q, want %q (digest of wave string %q)", key1, want, s)
	}
}

func TestWaveKey_HasReadablePrefixAndFixedLengthDigest(t *testing.T) {
	key := WaveKey("parent-1", []string{"child-1"})
	const prefix = "task_children_completed:parent-1:"
	if len(key) <= len(prefix) {
		t.Fatalf("WaveKey() = %q, want prefix %q followed by a digest", key, prefix)
	}
	if key[:len(prefix)] != prefix {
		t.Fatalf("WaveKey() = %q, want prefix %q", key, prefix)
	}
	digest := key[len(prefix):]
	const sha256HexLen = 64
	if len(digest) != sha256HexLen {
		t.Fatalf("WaveKey() digest length = %d, want %d (lowercase hex sha256)", len(digest), sha256HexLen)
	}
}

func TestWaveKey_DifferentMemberSetsDiffer(t *testing.T) {
	a := WaveKey("p", []string{"1", "2"})
	b := WaveKey("p", []string{"1", "3"})
	if a == b {
		t.Fatalf("expected different member sets to produce different wave keys, got %q for both", a)
	}
}

func TestWaveKey_SeparatorCannotCollideAcrossDifferentSplits(t *testing.T) {
	// A pipe-joined id list must not let two different (parent, members)
	// splits collide into the same wave string, the way a naive
	// concatenation without a separator could.
	a := WaveString("p1", []string{"ab", "c"})
	b := WaveString("p1a", []string{"b", "c"})
	if a == b {
		t.Fatalf("expected distinguishable splits to produce different wave strings, got %q for both", a)
	}
}
