import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { i18n } from "@/lib/i18n";
import { RepositoryBranchTemplateHelp } from "./repository-branch-template-help";

const TRIGGER_LABEL = "Branch template placeholders";

// Restoring the locale is load-bearing: `changeLanguage` mutates the shared
// instance `vitest.setup.ts` initializes, so leaving it on pseudo would leak
// into every test that runs after this file.
afterEach(async () => {
  cleanup();
  await i18n.changeLanguage("en");
});

/**
 * The six placeholder tokens are substituted by exact string match in
 * `RenderTaskBranchName` (apps/backend/internal/worktree/config.go), so they must
 * stay in English while the description beside each one is translated. The
 * description now arrives through a catalog key held in the table, which means a
 * mistyped key renders the key itself and nothing fails — these assertions pin
 * the token-to-description pairing and the interpolated example values.
 */
function openHelp() {
  render(<RepositoryBranchTemplateHelp />);
  // HoverCard content is only mounted while open; the trigger is a button, so
  // focus is enough in happy-dom.
  screen.getByRole("button", { name: TRIGGER_LABEL }).focus();
}

/**
 * The trigger is icon-only, so its `aria-label` is the ONLY copy a screen-reader
 * user gets from it — and attribute copy has no second check. The pseudo-locale
 * oracle walks text nodes (`docs/i18n.md`, "It cannot see copy that is not a
 * text node"), so reverting this to an English literal leaves no trace on
 * screen. The locale-switch assertion below covers the two failures lint cannot:
 * a hardcoded literal, and a `t()` frozen at module scope. Both render the same
 * English forever; only a switch tells them apart.
 */
describe("RepositoryBranchTemplateHelp", () => {
  it("labels the trigger for screen readers", () => {
    render(<RepositoryBranchTemplateHelp />);

    expect(screen.getByRole("button", { name: TRIGGER_LABEL })).toBeTruthy();
  });

  it("resolves the trigger label through the catalog on a locale switch", async () => {
    render(<RepositoryBranchTemplateHelp />);
    await i18n.changeLanguage("pseudo");

    const label = screen.getByRole("button").getAttribute("aria-label") ?? "";
    expect(label).not.toBe(TRIGGER_LABEL);
    expect(label.length).toBeGreaterThan(0);
  });

  it("pairs each untranslated token with its description and example", async () => {
    openHelp();

    const term = await screen.findByText("{title}");
    const description = term.nextElementSibling;
    expect(description?.textContent).toBe(
      "Task title sanitized to lowercase ASCII, hyphen-separated, max 20 chars. Example: fix-login-flow.",
    );
  });

  it("keeps the ticket examples as interpolated values", async () => {
    openHelp();

    const term = await screen.findByText("{ticket}");
    expect(term.nextElementSibling?.textContent).toBe(
      "Task identifier first; otherwise Jira, Linear, GitHub issue, or GitHub PR metadata. Examples: KAN-123, #42.",
    );
  });

  it("renders the alias description that carries no example", async () => {
    openHelp();

    const term = await screen.findByText("{issue_key}");
    expect(term.nextElementSibling?.textContent).toBe(
      "Alias for ticket. Use whichever name reads better in your template.",
    );
  });

  it("keeps the literal-prefix example inside a code element", async () => {
    openHelp();

    const code = await screen.findByText("feature/{ticket}-{title}");
    expect(code.tagName).toBe("CODE");
    expect(code.parentElement?.textContent).toBe(
      "Write literal prefixes directly, for example feature/{ticket}-{title}.",
    );
  });
});
