import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createOfficeSlice } from "./office-slice";
import type { OfficeSlice, ProviderHealth, RouteAttempt, WorkspaceRouting } from "./types";

function makeStore() {
  return create<OfficeSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createOfficeSlice as any)(...a) })),
  );
}

const CLAUDE = "claude-acp";
const CODEX = "codex-acp";
const WS_1 = "ws-1";

function makeConfig(): WorkspaceRouting {
  return {
    enabled: true,
    provider_order: [CLAUDE, CODEX],
    default_tier: "balanced",
    provider_profiles: {
      [CLAUDE]: { tier_map: { balanced: "sonnet" } },
      [CODEX]: { tier_map: { balanced: "gpt-5.4" } },
    },
  };
}

function makeHealth(provider: string, state: ProviderHealth["state"]): ProviderHealth {
  return {
    provider_id: provider,
    scope: "provider",
    scope_value: "",
    state,
    backoff_step: 0,
  };
}

function makeAttempt(seq: number, providerId: string): RouteAttempt {
  return {
    seq,
    provider_id: providerId,
    model: "m",
    tier: "balanced",
    outcome: "launched",
    started_at: "2026-05-10T12:00:00Z",
  };
}

describe("office routing store actions", () => {
  it("setWorkspaceRouting + setKnownProviders", () => {
    const store = makeStore();
    store.getState().setKnownProviders([CLAUDE, CODEX]);
    store.getState().setWorkspaceRouting(WS_1, makeConfig());
    expect(store.getState().office.routing.knownProviders).toHaveLength(2);
    expect(store.getState().office.routing.byWorkspace[WS_1]?.enabled).toBe(true);
  });

  it("setWorkspaceRouting(undefined) clears the cache (used by routing.settings_updated)", () => {
    const store = makeStore();
    store.getState().setWorkspaceRouting(WS_1, makeConfig());
    store.getState().setWorkspaceRouting(WS_1, undefined);
    expect(store.getState().office.routing.byWorkspace[WS_1]).toBeUndefined();
  });

  it("upsertProviderHealth replaces a row matching (provider, scope, scope_value)", () => {
    const store = makeStore();
    store.getState().setProviderHealth(WS_1, [makeHealth(CLAUDE, "healthy")]);
    store.getState().upsertProviderHealth(WS_1, makeHealth(CLAUDE, "degraded"));
    const list = store.getState().office.providerHealth.byWorkspace[WS_1];
    expect(list).toHaveLength(1);
    expect(list?.[0].state).toBe("degraded");
  });

  it("upsertProviderHealth appends new (provider) rows", () => {
    const store = makeStore();
    store.getState().setProviderHealth(WS_1, [makeHealth(CLAUDE, "healthy")]);
    store.getState().upsertProviderHealth(WS_1, makeHealth(CODEX, "degraded"));
    expect(store.getState().office.providerHealth.byWorkspace[WS_1]).toHaveLength(2);
  });

  it("appendRunAttempt appends and dedupes by seq", () => {
    const store = makeStore();
    store.getState().appendRunAttempt("run-1", makeAttempt(1, CLAUDE));
    store.getState().appendRunAttempt("run-1", makeAttempt(2, CODEX));
    store
      .getState()
      .appendRunAttempt("run-1", { ...makeAttempt(2, CODEX), outcome: "failed_other" });
    const list = store.getState().office.runAttempts.byRunId["run-1"];
    expect(list).toHaveLength(2);
    expect(list?.[1].outcome).toBe("failed_other");
  });

  it("setRoutingPreview overwrites the per-workspace agent list", () => {
    const store = makeStore();
    store.getState().setRoutingPreview(WS_1, [
      {
        agent_id: "a-1",
        agent_name: "CEO",
        tier_source: "inherit",
        effective_tier: "balanced",
        fallback_chain: [],
        missing: [],
        degraded: false,
      },
    ]);
    expect(store.getState().office.routing.preview.byWorkspace[WS_1]).toHaveLength(1);
  });
});
