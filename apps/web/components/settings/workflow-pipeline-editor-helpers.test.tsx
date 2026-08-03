import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { i18n } from "@/lib/i18n";
import type { WorkflowStep } from "@/lib/types/http";
import {
  getChildrenCompletedTransitionType,
  HelpTip,
  PROMPT_TEMPLATES,
  STEP_COLORS,
} from "./workflow-pipeline-editor-helpers";

/**
 * Attribute-borne copy has no second check: the pseudo-locale oracle walks text
 * nodes, so an `aria-label` reverted to an English literal leaves no trace on
 * screen and nothing downstream catches it (see docs/i18n.md, "It cannot see
 * copy that is not a text node"). `HelpTip` is the densest attribute-copy site
 * on this surface — every help affordance in the pipeline editor is one — so its
 * default label is pinned here rather than left to lint alone.
 */
describe("HelpTip aria-label", () => {
  // `cleanup()` is load-bearing, not redundant: vitest.config.ts does not set
  // `globals: true`, so `afterEach` is not on globalThis and RTL never
  // auto-registers its own cleanup. Without this, the two renders below collide
  // on `[data-testid="tip"]`.
  afterEach(async () => {
    cleanup();
    await i18n.changeLanguage("en");
  });

  it("resolves its default label through the catalog", () => {
    render(<HelpTip text="body" testId="tip" />);
    expect(screen.getByTestId("tip").getAttribute("aria-label")).toBe("More information");
  });

  it("follows a locale switch rather than being frozen at boot", async () => {
    render(<HelpTip text="body" testId="tip" />);
    await i18n.changeLanguage("pseudo");
    const label = screen.getByTestId("tip").getAttribute("aria-label") ?? "";
    expect(label).not.toBe("More information");
    expect(label).toMatch(/[À-ɏ]/);
  });

  // `ariaLabel` short-circuits the default (`ariaLabel ?? t(...)`), so the
  // override is whatever the caller passes — every call site in this surface
  // passes an already-translated string.
  it("lets a caller supply a pre-translated label override", () => {
    render(<HelpTip text="body" testId="tip" ariaLabel="Custom label" />);
    expect(screen.getByTestId("tip").getAttribute("aria-label")).toBe("Custom label");
  });
});

/**
 * Both tables are the guard's documented blind spot — a literal assigned to a
 * SCREAMING_CASE identifier is skipped entirely — so their shape is asserted
 * here: every entry carries a catalog key that resolves, and neither the
 * persisted `value` nor the persisted `prompt` is translated.
 */
describe("step colour and prompt-template tables", () => {
  it("gives every colour a resolving label key and a Tailwind value", () => {
    for (const color of STEP_COLORS) {
      expect(color.value).toMatch(/^bg-[a-z]+-\d{3}$/);
      expect(color.labelKey.startsWith("workflows:")).toBe(true);
      expect(i18n.t(color.labelKey)).not.toBe(color.labelKey);
    }
  });

  it("translates template labels but never their persisted prompt bodies", () => {
    for (const template of PROMPT_TEMPLATES) {
      expect(i18n.t(template.labelKey)).not.toBe(template.labelKey);
      // The prompt is written into WorkflowStep.prompt and sent to the agent
      // verbatim, so it must stay a plain English literal.
      expect(template.prompt).toMatch(/^[\s\S]*[A-Za-z]{4,}[\s\S]*$/);
      expect(template.prompt.startsWith("workflows:")).toBe(false);
    }
  });
});

describe("workflow pipeline editor helpers", () => {
  it("reads the all child tasks complete transition from workflow step events", () => {
    const step = {
      events: {
        on_children_completed: [{ type: "move_to_step", config: { step_id: "done-step" } }],
      },
    } as WorkflowStep;

    expect(getChildrenCompletedTransitionType(step)).toBe("move_to_step");
  });

  it("defaults all child tasks complete to none when no transition is configured", () => {
    expect(getChildrenCompletedTransitionType({ events: {} } as WorkflowStep)).toBe("none");
  });
});
