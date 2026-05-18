import { describe, it, expect } from "vitest";
import {
  CHAT_PANEL_FALLBACK_LABEL,
  collectSessionIdsForEnv,
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
 * referred to a previously-deleted task's session. The fix filters session
 * panels against the env's currently-known session ids at restore time;
 * this helper extracts that set from the appStore.
 */
describe("collectSessionIdsForEnv", () => {
  it("returns only session ids whose mapping matches the env", () => {
    const state = {
      environmentIdBySessionId: {
        "sess-1": "env-A",
        "sess-2": "env-A",
        "sess-3": "env-B",
      },
    };
    expect(collectSessionIdsForEnv(state, "env-A")).toEqual(new Set(["sess-1", "sess-2"]));
    expect(collectSessionIdsForEnv(state, "env-B")).toEqual(new Set(["sess-3"]));
  });

  it("returns an empty set when no sessions map to the env (e.g. brand-new task)", () => {
    const state = { environmentIdBySessionId: { "sess-1": "env-A" } };
    expect(collectSessionIdsForEnv(state, "env-new")).toEqual(new Set());
  });

  it("returns an empty set when the mapping is empty", () => {
    expect(collectSessionIdsForEnv({ environmentIdBySessionId: {} }, "env-A")).toEqual(new Set());
  });
});
