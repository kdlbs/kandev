import { describe, it, expect } from "vitest";
import type { ReactElement } from "react";
import { t } from "@/lib/i18n";
import { renderPanel, resolveChatPanelTitle } from "./dockview-panel-content";
import { DESKTOP_COMPONENT_NAMES, DESKTOP_VALID_COMPONENTS } from "./dockview-desktop-layout";
import { RENDERABLE_COMPONENT_NAMES } from "@/lib/state/layout-manager/renderable-components";

/** The element type `renderPanel` resolves a component to. A name with no
 *  renderer resolves to the unknownPanel placeholder, whose type is `div`. */
function renderedElementType(component: string): unknown {
  return (renderPanel("panel", component, {}) as ReactElement).type;
}

describe("dockview desktop layout registry", () => {
  it("registers exactly the renderable component names", () => {
    expect(DESKTOP_COMPONENT_NAMES).toEqual([...RENDERABLE_COMPONENT_NAMES]);
  });

  it("accepts every component the desktop renderer knows", () => {
    for (const component of RENDERABLE_COMPONENT_NAMES) {
      expect(DESKTOP_VALID_COMPONENTS.has(component)).toBe(true);
    }
  });

  it("resolves every renderable name to a real renderer, not the unknownPanel placeholder", () => {
    // The placeholder is the fallback branch of `renderPanel`, so this pairing
    // is what gives the loop below its power: "renders something" would pass
    // for a name with no renderer at all.
    expect(renderedElementType("component-with-no-renderer")).toBe("div");
    for (const component of RENDERABLE_COMPONENT_NAMES) {
      expect(renderedElementType(component), component).not.toBe("div");
    }
  });

  it("resolves the legacy aliases to their canonical target", () => {
    expect(renderedElementType("diff-files")).toBe(renderedElementType("changes"));
    expect(renderedElementType("all-files")).toBe(renderedElementType("files"));
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
