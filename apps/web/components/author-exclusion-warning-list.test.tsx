import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { AuthorExclusionWarningList } from "./author-exclusion-warning-list";
import type { AuthorExclusionWarning } from "@/lib/types/author-exclusion";

const base = { workflow_name: "Office Review", step_name: "Review", role: "reviewer" };

describe("AuthorExclusionWarningList", () => {
  afterEach(cleanup);

  it("renders nothing without warnings", () => {
    const { container } = render(<AuthorExclusionWarningList warnings={[]} />);
    expect(container.innerHTML).toBe("");
  });

  it("states what is true, why it is bad, and the fix on the routing page", () => {
    const warnings: AuthorExclusionWarning[] = [
      { ...base, code: "routing_disabled" },
      { ...base, code: "single_provider", provider_id: "claude-acp" },
      { ...base, code: "agent_single_provider", provider_id: "codex-acp", agent_name: "QA" },
      { code: "check_failed" },
    ];
    const label = (id: string) => (id === "codex-acp" ? "Codex (ACP)" : id);
    render(
      <AuthorExclusionWarningList warnings={warnings} formatProvider={label} surface="routing" />,
    );
    expect(screen.getByText("Review on a different provider can't work as set up")).toBeTruthy();
    const items = screen.getAllByRole("listitem").map((li) => li.textContent);
    expect(items).toEqual([
      "Office Review workflow, Review step (Reviewer): Automatic provider fallback is off, so Review on a different provider is ignored. Reviews may run on the provider that did the work, so they are not an independent check. Turn on Automatic provider fallback, or turn off Review on a different provider for this step.",
      "Office Review workflow, Review step (Reviewer): claude-acp is the only provider, so the reviewer can never use a different one from the work. Reviews will wait until you act. Add a second provider, or turn off Review on a different provider for this step.",
      'Office Review workflow, Review step (Reviewer): Agent "QA" can only use Codex (ACP), so its reviews wait whenever the work ran on Codex (ACP). Give "QA" another provider in its routing settings, or turn off Review on a different provider for this step.',
      "Review on a different provider could not be checked. Reload the page to try again.",
    ]);
  });

  it("points at Office > Routing when shown elsewhere", () => {
    render(
      <AuthorExclusionWarningList
        warnings={[{ ...base, code: "single_provider", provider_id: "claude-acp" }]}
      />,
    );
    expect(screen.getByRole("listitem").textContent).toBe(
      "Office Review workflow, Review step (Reviewer): claude-acp is the only provider, so the reviewer can never use a different one from the work. Reviews will wait until you act. Add a second provider in Office > Routing, or turn off Review on a different provider for this step there.",
    );
  });
});
