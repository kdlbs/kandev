import { describe, it, expect } from "vitest";
import {
  CHAT_PANEL_FALLBACK_LABEL,
  collectPhantomSessionIdsForEnv,
  resolveChatPanelTitle,
} from "./dockview-desktop-layout";

/**
 * Regression: the generic "chat" placeholder dockview panel used to fall back
 * to the literal "Agent" label even when the active session's agent profile
 * was loaded (e.g. "Opus"). The bug was a stale `isSessionTab && agentLabel`
 * gate inside `useChatSessionTitle` that suppressed the agent label for the
 * non-session-scoped placeholder. The pure resolver below is the single place
 * the gate would have to be re-introduced, so this test pins the behavior.
 */
describe("resolveChatPanelTitle", () => {
  it("returns the agent label when one is provided", () => {
    expect(resolveChatPanelTitle("Opus")).toBe("Opus");
  });

  it("falls back to the generic 'Agent' label when null", () => {
    expect(resolveChatPanelTitle(null)).toBe(CHAT_PANEL_FALLBACK_LABEL);
  });

  it("falls back to the generic 'Agent' label when undefined", () => {
    expect(resolveChatPanelTitle(undefined)).toBe(CHAT_PANEL_FALLBACK_LABEL);
  });

  it("falls back to the generic 'Agent' label when the agent label is empty", () => {
    expect(resolveChatPanelTitle("")).toBe(CHAT_PANEL_FALLBACK_LABEL);
  });

  it("uses the agent label verbatim — does not coerce or relabel valid names", () => {
    for (const name of ["Mock", "Claude Code", "GPT-5", "amp"]) {
      expect(resolveChatPanelTitle(name)).toBe(name);
    }
  });
});

/**
 * Regression: an env-layout could be restored with a session panel whose id
 * referred to a previously-deleted task's session. The fix strips session
 * panels we KNOW belong to a different env; sessions not yet mapped in the
 * store are preserved (they may still be loading via WS).
 */
describe("collectPhantomSessionIdsForEnv", () => {
  it("returns session ids whose mapping is a different env", () => {
    const state = {
      environmentIdBySessionId: {
        "sess-1": "env-A",
        "sess-2": "env-A",
        "sess-3": "env-B",
      },
    };
    expect(collectPhantomSessionIdsForEnv(state, "env-A")).toEqual(new Set(["sess-3"]));
    expect(collectPhantomSessionIdsForEnv(state, "env-B")).toEqual(new Set(["sess-1", "sess-2"]));
  });

  it("returns every mapped session as phantom when the env has no own sessions yet", () => {
    const state = { environmentIdBySessionId: { "sess-1": "env-A" } };
    expect(collectPhantomSessionIdsForEnv(state, "env-new")).toEqual(new Set(["sess-1"]));
  });

  it("does NOT classify a session as a phantom when its mapping is absent (still loading via WS)", () => {
    // A session id present in a saved layout but not in environmentIdBySessionId
    // could be a not-yet-arrived session for this very env. Keep it; reconcile
    // will clean it up later if it really is stale.
    const state = { environmentIdBySessionId: {} };
    expect(collectPhantomSessionIdsForEnv(state, "env-A")).toEqual(new Set());
  });
});
