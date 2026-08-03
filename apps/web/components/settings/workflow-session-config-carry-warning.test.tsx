import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { i18n } from "@/lib/i18n";
import type { SessionConfigCarryWarning } from "@/lib/workflows/session-config-carry-analysis";
import { SessionConfigCarryWarningPanel } from "./workflow-session-config-carry-warning";

/**
 * The carry-forward sentence used to be baked into the analyzer's result, which
 * froze it at the locale of the last analysis: the panel's own labels would
 * follow a locale switch while the sentence stayed behind. It is resolved here
 * now, so these tests pin both the interpolation and that it tracks the locale.
 */

const WARNING: SessionConfigCarryWarning = {
  agentName: "codex",
  sourceStepId: "step-1",
  sourceStepName: "In Progress",
  configOptions: {},
};

function renderPanel() {
  render(
    <SessionConfigCarryWarningPanel warnings={[WARNING]} onChoose={vi.fn()} readOnly={false} />,
  );
}

describe("SessionConfigCarryWarningPanel", () => {
  afterEach(async () => {
    cleanup();
    await i18n.changeLanguage("en");
  });

  it("interpolates the source step and agent names into the sentence", () => {
    renderPanel();
    expect(
      screen.getByText(
        "Settings changed in In Progress may carry into this step for codex. Choose keep, restore original, or set new values.",
      ),
    ).toBeTruthy();
  });

  it("re-resolves the sentence when the locale changes while mounted", async () => {
    renderPanel();
    const english = screen.getByText(/Settings changed in In Progress/);
    expect(english).toBeTruthy();

    await i18n.changeLanguage("pseudo");

    // The names are interpolated values, so they survive verbatim; the copy
    // around them must have changed, which is what a baked-in message could not do.
    const pseudo = screen.getByText(/In Progress/);
    expect(pseudo.textContent).toContain("In Progress");
    expect(pseudo.textContent).toContain("codex");
    expect(pseudo.textContent).not.toContain("Settings changed in");
  });
});
