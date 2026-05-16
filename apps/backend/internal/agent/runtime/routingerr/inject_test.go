package routingerr

import "testing"

func TestLoadInjectionFromEnv_Empty(t *testing.T) {
	t.Setenv("KANDEV_PROVIDER_FAILURES", "")
	got := LoadInjectionFromEnv()
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

func TestLoadInjectionFromEnv_SingleEntry(t *testing.T) {
	t.Setenv("KANDEV_PROVIDER_FAILURES", "claude-acp:quota_limited")
	got := LoadInjectionFromEnv()
	if got["claude-acp"] != CodeQuotaLimited {
		t.Fatalf("got %v", got)
	}
}

func TestLoadInjectionFromEnv_MultipleEntries(t *testing.T) {
	t.Setenv("KANDEV_PROVIDER_FAILURES", "claude-acp:quota_limited, codex-acp:auth_required ,opencode-acp:rate_limited")
	got := LoadInjectionFromEnv()
	if got["claude-acp"] != CodeQuotaLimited {
		t.Fatalf("claude entry: %v", got)
	}
	if got["codex-acp"] != CodeAuthRequired {
		t.Fatalf("codex entry: %v", got)
	}
	if got["opencode-acp"] != CodeRateLimited {
		t.Fatalf("opencode entry: %v", got)
	}
}

func TestLoadInjectionFromEnv_Malformed(t *testing.T) {
	t.Setenv("KANDEV_PROVIDER_FAILURES", "no-colon,:leading,trailing:,valid:quota_limited,,  ")
	got := LoadInjectionFromEnv()
	if len(got) != 1 || got["valid"] != CodeQuotaLimited {
		t.Fatalf("got %v", got)
	}
}

func TestLoadInjectionFromEnv_CaseSensitive(t *testing.T) {
	t.Setenv("KANDEV_PROVIDER_FAILURES", "Claude-ACP:quota_limited")
	got := LoadInjectionFromEnv()
	if got["Claude-ACP"] != CodeQuotaLimited {
		t.Fatalf("expected case-preserved key, got %v", got)
	}
	if _, ok := got["claude-acp"]; ok {
		t.Fatalf("lookup must be case-sensitive")
	}
}
