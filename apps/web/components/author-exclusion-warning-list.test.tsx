import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { AuthorExclusionWarningList } from "./author-exclusion-warning-list";
import type { AuthorExclusionWarning } from "@/lib/types/author-exclusion";

const CLAUDE = "claude-acp";
const base = { workflow_name: "Office Review", step_name: "Review", role: "reviewer" };

function groupTexts() {
  return screen.getAllByTestId("author-exclusion-warning-group").map((group) => ({
    problem: within(group).getByTestId("author-exclusion-warning-problem").textContent,
    steps: within(group)
      .queryAllByRole("listitem")
      .map((li) => li.textContent),
    fix: within(group).getByTestId("author-exclusion-warning-fix").textContent,
  }));
}

describe("AuthorExclusionWarningList", () => {
  afterEach(cleanup);

  it("renders nothing without warnings", () => {
    const { container } = render(<AuthorExclusionWarningList warnings={[]} />);
    expect(container.innerHTML).toBe("");
  });

  it("states what is true, the affected step, and the fix on the routing page", () => {
    const warnings: AuthorExclusionWarning[] = [
      { ...base, code: "routing_disabled" },
      { ...base, code: "single_provider", provider_id: CLAUDE },
      { ...base, code: "agent_single_provider", provider_id: "codex-acp", agent_name: "QA" },
      { ...base, code: "agent_bound_profile", agent_name: "Lead" },
      { code: "check_failed" },
    ];
    const label = (id: string) => (id === "codex-acp" ? "Codex (ACP)" : id);
    render(
      <AuthorExclusionWarningList warnings={warnings} formatProvider={label} surface="routing" />,
    );
    expect(screen.getByText("Review on a different provider can't work as set up")).toBeTruthy();
    const step = "Office Review workflow, Review step (Reviewer)";
    expect(groupTexts()).toEqual([
      {
        problem:
          "Automatic provider fallback is off, so Review on a different provider is ignored. Reviews may run on the provider that did the work, so they are not an independent check.",
        steps: [step],
        fix: "Turn on Automatic provider fallback, or turn off Review on a different provider for this step.",
      },
      {
        problem:
          "claude-acp is the only provider, so reviews can never run on a different one from the work. Reviews will wait until you act.",
        steps: [step],
        fix: "Add a second provider, or turn off Review on a different provider for this step.",
      },
      {
        problem:
          'Agent "QA" can only use Codex (ACP), so its reviews wait whenever the work ran on Codex (ACP).',
        steps: [step],
        fix: 'Give "QA" another provider in its routing settings, or turn off Review on a different provider for this step.',
      },
      {
        problem:
          'Agent "Lead" uses its own execution profile, which Review on a different provider does not cover, so its reviews can run on the provider that did the work.',
        steps: [step],
        fix: 'Clear the Execution profile in "Lead"\'s configuration so it follows Office provider routing.',
      },
      {
        problem: "Review on a different provider could not be checked.",
        steps: [],
        fix: "Reload the page to try again.",
      },
    ]);
  });

  it("shows one warning for steps that share a cause, with a toggle past three steps", () => {
    const warnings: AuthorExclusionWarning[] = ["A", "B", "C", "D", "E"].map((wf) => ({
      code: "single_provider",
      workflow_name: wf,
      step_name: "Review",
      role: "reviewer",
      provider_id: CLAUDE,
    }));
    render(<AuthorExclusionWarningList warnings={warnings} surface="routing" />);

    const [group] = groupTexts();
    expect(screen.getAllByTestId("author-exclusion-warning-group")).toHaveLength(1);
    expect(group.steps).toEqual([
      "A workflow, Review step (Reviewer)",
      "B workflow, Review step (Reviewer)",
      "C workflow, Review step (Reviewer)",
    ]);
    expect(group.fix).toBe(
      "Add a second provider, or turn off Review on a different provider for these steps.",
    );

    fireEvent.click(screen.getByRole("button", { name: "Show all 5 steps" }));
    expect(screen.getAllByRole("listitem")).toHaveLength(5);
    fireEvent.click(screen.getByRole("button", { name: "Show fewer" }));
    expect(screen.getAllByRole("listitem")).toHaveLength(3);
  });

  it("points at Office > Routing when shown elsewhere", () => {
    render(
      <AuthorExclusionWarningList
        warnings={[
          { ...base, code: "single_provider", provider_id: CLAUDE },
          { ...base, workflow_name: "Other", code: "single_provider", provider_id: CLAUDE },
        ]}
      />,
    );
    expect(screen.getByTestId("author-exclusion-warning-fix").textContent).toBe(
      "Add a second provider in Office > Routing, or turn off Review on a different provider for these steps there.",
    );
    expect(screen.queryByRole("button")).toBeNull();
  });
});
