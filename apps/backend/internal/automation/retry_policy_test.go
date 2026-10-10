package automation

import "testing"

func TestNormalizeRetryPolicyDefaultsDisabledValues(t *testing.T) {
	policy, err := NormalizeRetryPolicy(RetryPolicy{Mode: RetryModeDisabled})
	if err != nil {
		t.Fatalf("normalize disabled policy: %v", err)
	}
	if policy.Mode != RetryModeDisabled || policy.MaxRetries != "0" || policy.DelaySeconds != "0" ||
		policy.Backoff != RetryBackoffFixed || policy.HistoryMode != RetryHistoryAttempts {
		t.Fatalf("unexpected normalized policy: %#v", policy)
	}
}

func TestNormalizeRetryPolicyValidFiniteValues(t *testing.T) {
	policy, err := NormalizeRetryPolicy(RetryPolicy{
		Mode: RetryModeFinite, MaxRetries: "2", DelaySeconds: "30",
		Backoff: RetryBackoffExponential, HistoryMode: RetryHistoryTimeline,
	})
	if err != nil {
		t.Fatalf("normalize finite policy: %v", err)
	}
	if policy.MaxRetries != "2" || policy.DelaySeconds != "30" ||
		policy.Backoff != RetryBackoffExponential || policy.HistoryMode != RetryHistoryTimeline {
		t.Fatalf("unexpected normalized finite policy: %#v", policy)
	}
}

func TestNormalizeRetryPolicyRejectsInvalidValues(t *testing.T) {
	cases := []RetryPolicy{
		{Mode: RetryModeFinite, MaxRetries: "0"},
		{Mode: RetryModeFinite, MaxRetries: "9223372036854775807"},
		{Mode: RetryModeFinite, MaxRetries: "1", DelaySeconds: "-1"},
		{Mode: "other", MaxRetries: "1"},
		{Mode: RetryModeFinite, MaxRetries: "1", Backoff: "other"},
		{Mode: RetryModeFinite, MaxRetries: "1", HistoryMode: "other"},
	}
	for _, tc := range cases {
		if _, err := NormalizeRetryPolicy(tc); err == nil {
			t.Errorf("NormalizeRetryPolicy(%#v) succeeded", tc)
		}
	}
}

func TestNormalizeRetryPolicyDisabledIgnoresRetryOnlyValues(t *testing.T) {
	policy, err := NormalizeRetryPolicy(RetryPolicy{
		Mode:         RetryModeDisabled,
		MaxRetries:   "99",
		DelaySeconds: "30",
		Backoff:      RetryBackoffExponential,
		HistoryMode:  RetryHistoryTimeline,
	})
	if err != nil {
		t.Fatalf("normalize disabled policy: %v", err)
	}
	want := RetryPolicy{
		Mode:         RetryModeDisabled,
		MaxRetries:   "0",
		DelaySeconds: "0",
		Backoff:      RetryBackoffFixed,
		HistoryMode:  RetryHistoryAttempts,
	}
	if policy != want {
		t.Fatalf("normalized disabled policy = %#v, want %#v", policy, want)
	}
}
