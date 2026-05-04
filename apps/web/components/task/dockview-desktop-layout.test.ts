import { describe, it, expect } from "vitest";
import { CHAT_PANEL_FALLBACK_LABEL, resolveChatPanelTitle } from "./dockview-desktop-layout";

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
