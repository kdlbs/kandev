package environment

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func noReveal(context.Context, Definition) (string, error) {
	return "", errors.New("reveal should not be called")
}

func mustResolve(t *testing.T, defs []Definition) (map[string]string, []OverrideRecord) {
	t.Helper()
	env, records, err := Resolve(context.Background(), defs, noReveal)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	return env, records
}

func conflictOf(t *testing.T, defs []Definition) *ConflictError {
	t.Helper()
	env, records, err := Resolve(context.Background(), defs, noReveal)
	if err == nil {
		t.Fatalf("Resolve() succeeded, want conflict; env=%#v", env)
	}
	if env != nil || records != nil {
		t.Fatalf("Resolve() on error: env=%#v records=%#v, want both nil", env, records)
	}
	var conflictErr *ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("error = %T %v, want *ConflictError", err, err)
	}
	return conflictErr
}

// --- Precedence: AC-1 through AC-8e ---

func TestResolve_ExecutorProfileBeatsAgentProfile(t *testing.T) { // AC-1
	env, records := mustResolve(t, []Definition{
		{Key: "K", Literal: "executor-value", Origin: OriginExecutorProfile},
		{Key: "K", Literal: "agent-value", Origin: OriginAgentProfile},
	})
	if env["K"] != "executor-value" {
		t.Fatalf("K = %q, want executor-value", env["K"])
	}
	if len(records) != 1 || records[0].WinningTier != TierAuthoritative {
		t.Fatalf("records = %#v", records)
	}
}

func TestResolve_ManagedRuntimeBeatsAgentProfile(t *testing.T) { // AC-2
	env, _ := mustResolve(t, []Definition{
		{Key: "K", Literal: "managed-value", Origin: OriginManagedRuntime},
		{Key: "K", Literal: "agent-value", Origin: OriginAgentProfile},
	})
	if env["K"] != "managed-value" {
		t.Fatalf("K = %q, want managed-value", env["K"])
	}
}

func TestResolve_ManagedCredentialsBeatsAgentProfile(t *testing.T) { // AC-3
	env, _ := mustResolve(t, []Definition{
		{Key: "K", Literal: "cred-value", Origin: OriginManagedCredentials},
		{Key: "K", Literal: "agent-value", Origin: OriginAgentProfile},
	})
	if env["K"] != "cred-value" {
		t.Fatalf("K = %q, want cred-value", env["K"])
	}
}

func TestResolve_AgentProfileBeatsManagedAgentDefaults(t *testing.T) { // AC-4
	env, records := mustResolve(t, []Definition{
		{Key: "K", Literal: "profile-value", Origin: OriginAgentProfile},
		{Key: "K", Literal: "default-value", Origin: OriginManagedAgentDefaults},
	})
	if env["K"] != "profile-value" {
		t.Fatalf("K = %q, want profile-value", env["K"])
	}
	if len(records) != 1 || records[0].WinningTier != TierProfileDefault {
		t.Fatalf("records = %#v", records)
	}
	if len(records[0].LosingOrigins) != 1 || records[0].LosingOrigins[0] != (OriginTier{Origin: OriginManagedAgentDefaults, Tier: TierAgentRuntimeDefault}) {
		t.Fatalf("losing origins = %#v", records[0].LosingOrigins)
	}
}

func TestResolve_TwoDifferingTier1OriginsConflict(t *testing.T) { // AC-5
	err := conflictOf(t, []Definition{
		{Key: "K", Literal: "a", Origin: OriginExecutorProfile},
		{Key: "K", Literal: "b", Origin: OriginManagedRuntime},
	})
	want := []string{OriginExecutorProfile, OriginManagedRuntime}
	if !reflect.DeepEqual(err.Origins, want) {
		t.Fatalf("origins = %#v, want %#v", err.Origins, want)
	}
}

// TestResolve_MergedIdentityClassifiesByStrongestOrigin pins AC-5a: a merged
// executor-profile/agent-profile identity (same literal) classifies Tier 1
// via its strongest contributing origin and conflicts against a differing
// managed-runtime literal, rather than being silently ranked Tier 2 by the
// surviving Definition.Origin field (which would sort to "agent profile").
func TestResolve_MergedIdentityClassifiesByStrongestOrigin(t *testing.T) {
	err := conflictOf(t, []Definition{
		{Key: "AGENT_MODEL", Literal: "shared", Origin: OriginExecutorProfile},
		{Key: "AGENT_MODEL", Literal: "shared", Origin: OriginAgentProfile},
		{Key: "AGENT_MODEL", Literal: "different", Origin: OriginManagedRuntime},
	})
	want := []string{OriginAgentProfile, OriginExecutorProfile, OriginManagedRuntime}
	if !reflect.DeepEqual(err.Origins, want) {
		t.Fatalf("origins = %#v, want %#v", err.Origins, want)
	}
}

// TestResolve_MergedIdentitySpansTiersAndWins pins AC-5b: the same merged
// pair, now facing only a differing Tier 3 literal, resolves to the merged
// value with WinningOrigins spanning both contributing tiers while
// WinningTier records only the strongest.
func TestResolve_MergedIdentitySpansTiersAndWins(t *testing.T) {
	env, records := mustResolve(t, []Definition{
		{Key: "K", Literal: "shared", Origin: OriginExecutorProfile},
		{Key: "K", Literal: "shared", Origin: OriginAgentProfile},
		{Key: "K", Literal: "default", Origin: OriginManagedAgentDefaults},
	})
	if env["K"] != "shared" {
		t.Fatalf("K = %q, want shared", env["K"])
	}
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	want := []string{OriginAgentProfile, OriginExecutorProfile}
	if !reflect.DeepEqual(records[0].WinningOrigins, want) {
		t.Fatalf("winning origins = %#v, want %#v", records[0].WinningOrigins, want)
	}
	if records[0].WinningTier != TierAuthoritative {
		t.Fatalf("winning tier = %v, want TierAuthoritative", records[0].WinningTier)
	}
}

// TestResolve_SameOriginAmbiguityWithinWinningTierConflicts pins AC-6: two
// differing definitions from the SAME Tier-1 origin, with no stronger tier
// present, still conflict exactly as today.
func TestResolve_SameOriginAmbiguityWithinWinningTierConflicts(t *testing.T) {
	err := conflictOf(t, []Definition{
		{Key: "K", Literal: "a", Origin: OriginExecutorProfile},
		{Key: "K", Literal: "b", Origin: OriginExecutorProfile},
	})
	want := []string{OriginExecutorProfile}
	if !reflect.DeepEqual(err.Origins, want) {
		t.Fatalf("origins = %#v, want %#v", err.Origins, want)
	}
}

func TestResolve_AgentProfileOnlyNoExecutorProfile(t *testing.T) { // AC-7
	env, records := mustResolve(t, []Definition{
		{Key: "K", Literal: "agent-value", Origin: OriginAgentProfile},
	})
	if env["K"] != "agent-value" || records != nil {
		t.Fatalf("env=%#v records=%#v", env, records)
	}
}

func TestResolve_MergedTier1PairBeatsDifferingTier2(t *testing.T) { // AC-8a
	env, records := mustResolve(t, []Definition{
		{Key: "K", Literal: "shared", Origin: OriginManagedRuntime},
		{Key: "K", Literal: "shared", Origin: OriginManagedCredentials},
		{Key: "K", Literal: "profile", Origin: OriginAgentProfile},
	})
	if env["K"] != "shared" {
		t.Fatalf("K = %q, want shared", env["K"])
	}
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	want := []string{OriginManagedCredentials, OriginManagedRuntime}
	if !reflect.DeepEqual(records[0].WinningOrigins, want) {
		t.Fatalf("winning origins = %#v, want %#v", records[0].WinningOrigins, want)
	}
	if records[0].WinningTier != TierAuthoritative {
		t.Fatalf("winning tier = %v", records[0].WinningTier)
	}
	wantLosing := []OriginTier{{Origin: OriginAgentProfile, Tier: TierProfileDefault}}
	if !reflect.DeepEqual(records[0].LosingOrigins, wantLosing) {
		t.Fatalf("losing origins = %#v, want %#v", records[0].LosingOrigins, wantLosing)
	}
}

func TestResolve_TwoDifferingTier1PlusTier2Conflicts(t *testing.T) { // AC-8b
	err := conflictOf(t, []Definition{
		{Key: "K", Literal: "a", Origin: OriginManagedRuntime},
		{Key: "K", Literal: "b", Origin: OriginManagedCredentials},
		{Key: "K", Literal: "c", Origin: OriginAgentProfile},
	})
	want := []string{OriginAgentProfile, OriginManagedCredentials, OriginManagedRuntime}
	if !reflect.DeepEqual(err.Origins, want) {
		t.Fatalf("origins = %#v, want %#v", err.Origins, want)
	}
}

func TestResolve_Tier1BeatsBothTier2AndTier3(t *testing.T) { // AC-8c
	_, records := mustResolve(t, []Definition{
		{Key: "K", Literal: "a", Origin: OriginManagedRuntime},
		{Key: "K", Literal: "b", Origin: OriginAgentProfile},
		{Key: "K", Literal: "c", Origin: OriginManagedAgentDefaults},
	})
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	want := []OriginTier{
		{Origin: OriginAgentProfile, Tier: TierProfileDefault},
		{Origin: OriginManagedAgentDefaults, Tier: TierAgentRuntimeDefault},
	}
	if !reflect.DeepEqual(records[0].LosingOrigins, want) {
		t.Fatalf("losing origins = %#v, want %#v", records[0].LosingOrigins, want)
	}
}

// TestResolve_LosingOriginsDedupedByOrigin pins AC-8d: one Tier-1 winner
// plus two agent-profile literals that disagree with each other still
// produces exactly ONE LosingOrigins entry and one origin string, never two.
func TestResolve_LosingOriginsDedupedByOrigin(t *testing.T) {
	_, records := mustResolve(t, []Definition{
		{Key: "K", Literal: "a", Origin: OriginManagedRuntime},
		{Key: "K", Literal: "b", Origin: OriginAgentProfile},
		{Key: "K", Literal: "c", Origin: OriginAgentProfile},
	})
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	if len(records[0].LosingOrigins) != 1 {
		t.Fatalf("losing origins = %#v, want exactly 1 entry", records[0].LosingOrigins)
	}
	want := OriginTier{Origin: OriginAgentProfile, Tier: TierProfileDefault}
	if records[0].LosingOrigins[0] != want {
		t.Fatalf("losing origin = %#v, want %#v", records[0].LosingOrigins[0], want)
	}
}

// TestResolve_WinningOriginsDedupedByOrigin pins AC-8e: one Tier-1 value
// contributed by two definitions sharing both Origin and definitionIdentity
// lists that origin only once in WinningOrigins.
func TestResolve_WinningOriginsDedupedByOrigin(t *testing.T) {
	_, records := mustResolve(t, []Definition{
		{Key: "K", Literal: "shared", Origin: OriginManagedRuntime, WorkspaceID: ""},
		{Key: "K", Literal: "shared", Origin: OriginManagedRuntime, WorkspaceID: ""},
		{Key: "K", Literal: "other", Origin: OriginAgentProfile},
	})
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	want := []string{OriginManagedRuntime}
	if !reflect.DeepEqual(records[0].WinningOrigins, want) {
		t.Fatalf("winning origins = %#v, want %#v", records[0].WinningOrigins, want)
	}
}

// --- Secret veto: AC-9 through AC-13 ---

func TestResolve_SecretVetoBlocksAcrossTiers(t *testing.T) { // AC-9, AC-10
	err := conflictOf(t, []Definition{
		{Key: "K", SecretID: "secret-1", Origin: OriginExecutorProfile},
		{Key: "K", Literal: "literal", Origin: OriginAgentProfile},
	})
	want := []string{OriginAgentProfile, OriginExecutorProfile}
	if !reflect.DeepEqual(err.Origins, want) {
		t.Fatalf("origins = %#v, want %#v", err.Origins, want)
	}
}

func TestResolve_SecretVetoSymmetric(t *testing.T) { // AC-11
	err := conflictOf(t, []Definition{
		{Key: "K", Literal: "literal", Origin: OriginExecutorProfile},
		{Key: "K", SecretID: "secret-1", Origin: OriginAgentProfile},
	})
	want := []string{OriginAgentProfile, OriginExecutorProfile}
	if !reflect.DeepEqual(err.Origins, want) {
		t.Fatalf("origins = %#v, want %#v", err.Origins, want)
	}
}

func TestResolve_SecretVetoBlocksRepositoryAgainstOtherOrigin(t *testing.T) { // AC-12
	err := conflictOf(t, []Definition{
		{Key: "K", SecretID: "secret-1", Origin: "repository app", WorkspaceID: "ws-1"},
		{Key: "K", Literal: "literal", Origin: OriginManagedRuntime},
	})
	want := []string{"repository app", OriginManagedRuntime}
	if len(err.Origins) != 2 {
		t.Fatalf("origins = %#v, want 2 entries", err.Origins)
	}
	got := append([]string(nil), err.Origins...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("origins = %#v, want %#v", err.Origins, want)
	}
}

// TestResolve_VetoEnumeratesEveryContributingOriginOfMergedSecretIdentity
// pins AC-12a: two secret-backed definitions sharing one SecretID merge into
// one identity, and the veto's error must still name BOTH their origins
// (not just the surviving Definition.Origin), alongside the differing
// literal's origin.
func TestResolve_VetoEnumeratesEveryContributingOriginOfMergedSecretIdentity(t *testing.T) {
	err := conflictOf(t, []Definition{
		{Key: "K", SecretID: "secret-1", Origin: "repository app", WorkspaceID: "ws-1"},
		{Key: "K", SecretID: "secret-1", Origin: OriginExecutorProfile},
		{Key: "K", Literal: "literal", Origin: OriginAgentProfile},
	})
	want := []string{OriginAgentProfile, OriginExecutorProfile, "repository app"}
	if !reflect.DeepEqual(err.Origins, want) {
		t.Fatalf("origins = %#v, want %#v", err.Origins, want)
	}
}

func TestResolve_ConflictErrorLeaksNoSecretValue(t *testing.T) { // AC-13
	_, _, err := Resolve(context.Background(), []Definition{
		{Key: "K", SecretID: "super-secret-id", Origin: "repository app", WorkspaceID: "ws-1"},
		{Key: "K", Literal: "super-secret-literal", Origin: OriginAgentProfile},
	}, noReveal)
	if err == nil {
		t.Fatal("want conflict error")
	}
	msg := err.Error()
	for _, leaked := range []string{"super-secret-id", "super-secret-literal"} {
		if strings.Contains(msg, leaked) {
			t.Fatalf("error %q leaks %q", msg, leaked)
		}
	}
}

// --- No regression for currently-succeeding tasks: AC-14 through AC-17 ---

func TestResolve_SingleDefinitionUnchanged(t *testing.T) { // AC-14
	env, records := mustResolve(t, []Definition{{Key: "K", Literal: "v", Origin: OriginAgentProfile}})
	if env["K"] != "v" || records != nil {
		t.Fatalf("env=%#v records=%#v", env, records)
	}
}

func TestResolve_IdenticalIdentitiesMergeWithoutOverride(t *testing.T) { // AC-15
	env, records := mustResolve(t, []Definition{
		{Key: "K", Literal: "v", Origin: OriginExecutorProfile},
		{Key: "K", Literal: "v", Origin: OriginAgentProfile},
	})
	if env["K"] != "v" {
		t.Fatalf("K = %q, want v", env["K"])
	}
	if records != nil {
		t.Fatalf("records = %#v, want nil — nothing was overridden", records)
	}
}

// --- Observability: AC-20 through AC-26 ---

func TestResolve_NoOverrideRecordWhenNoConflict(t *testing.T) { // AC-26
	_, records, err := Resolve(context.Background(), []Definition{
		{Key: "K", Literal: "v", Origin: OriginAgentProfile},
	}, noReveal)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if records != nil {
		t.Fatalf("records = %#v, want nil (not an empty non-nil slice)", records)
	}
}

// TestResolve_ErrorReturnsNoRecordsEvenAfterEarlierOverride pins AC-20a: a
// set where key A resolves by tier precedence and key B conflicts must
// return nil records alongside the error, not a partial record for A.
func TestResolve_ErrorReturnsNoRecordsEvenAfterEarlierOverride(t *testing.T) {
	env, records, err := Resolve(context.Background(), []Definition{
		{Key: "A", Literal: "a1", Origin: OriginExecutorProfile},
		{Key: "A", Literal: "a2", Origin: OriginAgentProfile},
		{Key: "B", Literal: "b1", Origin: OriginManagedRuntime},
		{Key: "B", Literal: "b2", Origin: OriginExecutorProfile},
	}, noReveal)
	if err == nil {
		t.Fatal("want conflict on key B")
	}
	if env != nil || records != nil {
		t.Fatalf("env=%#v records=%#v, want both nil", env, records)
	}
}

// TestResolve_ErrorReturnsNoRecordsWhenSecretRevealFailsAfterOverride pins
// the second AC-20a branch: key A overrides, then a secret for key B fails
// to reveal — no records must leak through even though A already resolved.
func TestResolve_ErrorReturnsNoRecordsWhenSecretRevealFailsAfterOverride(t *testing.T) {
	failingReveal := func(context.Context, Definition) (string, error) {
		return "", errors.New("reveal failed")
	}
	env, records, err := Resolve(context.Background(), []Definition{
		{Key: "A", Literal: "a1", Origin: OriginExecutorProfile},
		{Key: "A", Literal: "a2", Origin: OriginAgentProfile},
		{Key: "B", SecretID: "secret-1", Origin: "repository app", WorkspaceID: "ws-1"},
	}, failingReveal)
	if err == nil {
		t.Fatal("want secret error")
	}
	var secretErr *SecretError
	if !errors.As(err, &secretErr) {
		t.Fatalf("error = %T %v, want *SecretError", err, err)
	}
	if env != nil || records != nil {
		t.Fatalf("env=%#v records=%#v, want both nil", env, records)
	}
}

func TestResolve_OverrideRecordCarriesNoSecretOrLiteral(t *testing.T) { // AC-21
	_, records := mustResolve(t, []Definition{
		{Key: "K", Literal: "secret-looking-value", Origin: OriginExecutorProfile},
		{Key: "K", Literal: "other", Origin: OriginAgentProfile},
	})
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	v := reflect.ValueOf(records[0])
	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		if field.Name == "WinningOrigins" || field.Name == "LosingOrigins" || field.Name == "WinningTier" || field.Name == "Key" {
			continue
		}
		t.Fatalf("OverrideRecord has unexpected field %q — every field must be identity/rank only", field.Name)
	}
}

// --- Determinism and ordering: AC-27 through AC-30 ---

func TestResolve_DeterministicAcrossRepeatedCalls(t *testing.T) { // AC-27
	defs := []Definition{
		{Key: "A", Literal: "a1", Origin: OriginExecutorProfile},
		{Key: "A", Literal: "a2", Origin: OriginAgentProfile},
		{Key: "B", Literal: "b1", Origin: OriginManagedRuntime},
		{Key: "B", Literal: "b2", Origin: OriginManagedAgentDefaults},
	}
	env1, records1, err1 := Resolve(context.Background(), defs, noReveal)
	env2, records2, err2 := Resolve(context.Background(), defs, noReveal)
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v, %v", err1, err2)
	}
	if !reflect.DeepEqual(env1, env2) {
		t.Fatalf("env1=%#v env2=%#v", env1, env2)
	}
	if !reflect.DeepEqual(records1, records2) {
		t.Fatalf("records1=%#v records2=%#v", records1, records2)
	}
}

func TestResolve_RecordsOrderedByKeyAscending(t *testing.T) { // AC-28
	_, records := mustResolve(t, []Definition{
		{Key: "ZKEY", Literal: "a", Origin: OriginExecutorProfile},
		{Key: "ZKEY", Literal: "b", Origin: OriginAgentProfile},
		{Key: "AKEY", Literal: "a", Origin: OriginExecutorProfile},
		{Key: "AKEY", Literal: "b", Origin: OriginAgentProfile},
	})
	if len(records) != 2 || records[0].Key != "AKEY" || records[1].Key != "ZKEY" {
		t.Fatalf("records = %#v, want AKEY then ZKEY", records)
	}
}

func TestResolve_SameResultRegardlessOfInputOrder(t *testing.T) { // AC-29, AC-30
	forward := []Definition{
		{Key: "K", Literal: "shared", Origin: OriginManagedRuntime},
		{Key: "K", Literal: "shared", Origin: OriginManagedCredentials},
		{Key: "K", Literal: "profile", Origin: OriginAgentProfile},
	}
	reversed := make([]Definition, len(forward))
	for i, d := range forward {
		reversed[len(forward)-1-i] = d
	}
	envA, recA := mustResolve(t, forward)
	envB, recB := mustResolve(t, reversed)
	if !reflect.DeepEqual(envA, envB) {
		t.Fatalf("envA=%#v envB=%#v", envA, envB)
	}
	if !reflect.DeepEqual(recA, recB) {
		t.Fatalf("recA=%#v recB=%#v", recA, recB)
	}
}

// TestResolve_WorkspaceIDTieBreakIsDeterministic pins AC-29b: two
// definitions sharing Key, Origin and SecretID but differing WorkspaceID
// merge, and the lexicographically smaller WorkspaceID's definition survives
// regardless of input order — observable via which reveal path is invoked.
func TestResolve_WorkspaceIDTieBreakIsDeterministic(t *testing.T) {
	reveal := func(_ context.Context, d Definition) (string, error) {
		return "workspace=" + d.WorkspaceID, nil
	}
	forward := []Definition{
		{Key: "K", SecretID: "secret-1", Origin: "repository app", WorkspaceID: "ws-b"},
		{Key: "K", SecretID: "secret-1", Origin: "repository app", WorkspaceID: "ws-a"},
	}
	reversed := []Definition{forward[1], forward[0]}

	envA, _, errA := Resolve(context.Background(), forward, reveal)
	envB, _, errB := Resolve(context.Background(), reversed, reveal)
	if errA != nil || errB != nil {
		t.Fatalf("errors: %v, %v", errA, errB)
	}
	if envA["K"] != "workspace=ws-a" || envB["K"] != "workspace=ws-a" {
		t.Fatalf("envA=%#v envB=%#v, want both to pick ws-a", envA, envB)
	}
}

// --- Nil, empty, and error behaviour: AC-31 through AC-37 ---

func TestResolve_EmptyDefinitionsReturnsEmptyMapNoError(t *testing.T) { // AC-31
	env, records, err := Resolve(context.Background(), nil, noReveal)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if env == nil || len(env) != 0 {
		t.Fatalf("env = %#v, want empty non-nil map", env)
	}
	if records != nil {
		t.Fatalf("records = %#v, want nil", records)
	}
}

func TestResolve_BlankKeyDefinitionsSkipped(t *testing.T) { // AC-32
	env, _ := mustResolve(t, []Definition{
		{Key: "", Literal: "v", Origin: OriginAgentProfile},
		{Key: "   ", Literal: "v", Origin: OriginAgentProfile},
		{Key: "K", Literal: "v", Origin: OriginAgentProfile},
	})
	if len(env) != 1 || env["K"] != "v" {
		t.Fatalf("env = %#v, want only K", env)
	}
}

func TestResolve_NilRevealOnSecretReturnsSecretError(t *testing.T) { // AC-33
	_, _, err := Resolve(context.Background(), []Definition{
		{Key: "K", SecretID: "secret-1", Origin: OriginExecutorProfile},
	}, nil)
	var secretErr *SecretError
	if !errors.As(err, &secretErr) {
		t.Fatalf("error = %T %v, want *SecretError", err, err)
	}
}

func TestResolve_RevealErrorReturnsSecretError(t *testing.T) { // AC-34
	_, _, err := Resolve(context.Background(), []Definition{
		{Key: "K", SecretID: "secret-1", Origin: OriginExecutorProfile},
	}, func(context.Context, Definition) (string, error) {
		return "", errors.New("boom")
	})
	var secretErr *SecretError
	if !errors.As(err, &secretErr) {
		t.Fatalf("error = %T %v, want *SecretError", err, err)
	}
}

func TestResolve_FirstConflictingKeyReported(t *testing.T) { // AC-36
	err := conflictOf(t, []Definition{
		{Key: "ZKEY", Literal: "a", Origin: OriginExecutorProfile},
		{Key: "ZKEY", Literal: "b", Origin: OriginManagedRuntime},
		{Key: "AKEY", Literal: "a", Origin: OriginExecutorProfile},
		{Key: "AKEY", Literal: "b", Origin: OriginManagedRuntime},
	})
	if err.Key != "AKEY" {
		t.Fatalf("key = %q, want AKEY", err.Key)
	}
}

func TestResolve_OnlyWinnerIsRevealed(t *testing.T) { // AC-37
	var revealed []string
	reveal := func(_ context.Context, d Definition) (string, error) {
		revealed = append(revealed, d.Origin)
		return "value", nil
	}
	// Two literal definitions plus one distinguishing secret-backed loser is
	// impossible under the veto, so this covers the ordinary case: a secret
	// winner is revealed and no other definition for the key is passed to
	// reveal.
	_, _, err := Resolve(context.Background(), []Definition{
		{Key: "K", SecretID: "secret-1", Origin: OriginExecutorProfile},
	}, reveal)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(revealed) != 1 || revealed[0] != OriginExecutorProfile {
		t.Fatalf("revealed = %#v", revealed)
	}
}

// --- Migration: AC-49 (resolver-level qualifiers) ---

func TestResolve_MigrationExecutorWinsWhenLiteralsDiffer(t *testing.T) {
	env, records := mustResolve(t, []Definition{
		{Key: "ANTHROPIC_BASE_URL", Literal: "https://executor.example", Origin: OriginExecutorProfile},
		{Key: "ANTHROPIC_BASE_URL", Literal: "https://agent.example", Origin: OriginAgentProfile},
	})
	if env["ANTHROPIC_BASE_URL"] != "https://executor.example" {
		t.Fatalf("env = %#v", env)
	}
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
}

func TestResolve_MigrationSecretSideStillBlocks(t *testing.T) {
	err := conflictOf(t, []Definition{
		{Key: "ANTHROPIC_BASE_URL", SecretID: "secret-1", Origin: OriginExecutorProfile},
		{Key: "ANTHROPIC_BASE_URL", Literal: "https://agent.example", Origin: OriginAgentProfile},
	})
	want := []string{OriginAgentProfile, OriginExecutorProfile}
	if !reflect.DeepEqual(err.Origins, want) {
		t.Fatalf("origins = %#v, want %#v", err.Origins, want)
	}
}

func TestResolve_MigrationIdenticalValuesStillMergeNoOverride(t *testing.T) {
	_, records := mustResolve(t, []Definition{
		{Key: "ANTHROPIC_BASE_URL", Literal: "https://same.example", Origin: OriginExecutorProfile},
		{Key: "ANTHROPIC_BASE_URL", Literal: "https://same.example", Origin: OriginAgentProfile},
	})
	if records != nil {
		t.Fatalf("records = %#v, want nil", records)
	}
}
