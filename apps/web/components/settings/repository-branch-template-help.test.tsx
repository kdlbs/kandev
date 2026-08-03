import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { RepositoryBranchTemplateHelp } from "./repository-branch-template-help";

afterEach(cleanup);

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
  screen.getByRole("button", { name: "Branch template placeholders" }).focus();
}

describe("RepositoryBranchTemplateHelp", () => {
  it("labels the trigger for screen readers", () => {
    render(<RepositoryBranchTemplateHelp />);

    expect(screen.getByRole("button", { name: "Branch template placeholders" })).toBeTruthy();
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
