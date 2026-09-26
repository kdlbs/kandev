import { describe, it, expect } from "vitest";
import { t } from "@/lib/i18n";
import { resolveChatPanelTitle } from "./dockview-panel-content";
import { DESKTOP_VALID_COMPONENTS } from "./dockview-desktop-layout";

describe("dockview desktop layout registry", () => {
  it("accepts the prompt-history component", () => {
    expect(DESKTOP_VALID_COMPONENTS.has("prompt-history")).toBe(true);
  });

  it("accepts every component the desktop renderer knows", () => {
    for (const component of ["chat", "plan", "todos", "files", "changes", "prompt-history"]) {
      expect(DESKTOP_VALID_COMPONENTS.has(component)).toBe(true);
    }
  });
});

/**
 * Regression: the generic "chat" placeholder dockview panel used to fall back
 * to the literal "Agent" label even when the active session's agent profile
 * was loaded (e.g. "Opus"). The bug was a stale `isSessionTab && agentLabel`
 * gate inside `useChatSessionTitle` that suppressed the agent label for the
 * non-session-scoped placeholder. The pure resolver imported here is the place
 * the gate would have to be re-introduced, so this test pins the behavior.
 *
 * The fallback is asserted through the catalog rather than as the literal
 * "Agent": what this test is about is WHICH value is chosen, not what that
 * value's English happens to be, and pinning the copy here would make an
 * ordinary wording change fail four unrelated assertions.
 */
const FALLBACK = () => t("task:panelAgent");

describe("resolveChatPanelTitle", () => {
  it("returns the agent label when one is provided", () => {
    expect(resolveChatPanelTitle("Opus", t)).toBe("Opus");
  });

  it("falls back to the generic 'Agent' label when null", () => {
    expect(resolveChatPanelTitle(null, t)).toBe(FALLBACK());
  });

  it("falls back to the generic 'Agent' label when undefined", () => {
    expect(resolveChatPanelTitle(undefined, t)).toBe(FALLBACK());
  });

  it("falls back to the generic 'Agent' label when the agent label is empty", () => {
    expect(resolveChatPanelTitle("", t)).toBe(FALLBACK());
  });

  it("uses the agent label verbatim — does not coerce or relabel valid names", () => {
    for (const name of ["Mock", "Claude Code", "GPT-5", "amp"]) {
      expect(resolveChatPanelTitle(name, t)).toBe(name);
    }
  });
});
