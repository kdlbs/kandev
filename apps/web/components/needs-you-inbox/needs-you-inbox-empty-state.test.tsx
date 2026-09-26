import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api/domains/clarification-inbox-api", () => ({
  listHiddenClarificationInbox: vi.fn().mockResolvedValue({ bundles: [], count: 0, total: 0 }),
  restoreClarificationInboxBundle: vi.fn(),
}));

let workspaceItems: Array<{ id: string; name: string }> = [{ id: "w1", name: "Kegmil V2" }];

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (s: any) => unknown) =>
    selector({
      workspaces: { activeId: "w1", items: workspaceItems },
      bumpNeedsYouInboxRefreshTick: vi.fn(),
    }),
}));

import { NeedsYouInboxEmptyState } from "./needs-you-inbox-empty-state";

afterEach(() => {
  cleanup();
  workspaceItems = [{ id: "w1", name: "Kegmil V2" }];
});

describe("NeedsYouInboxEmptyState", () => {
  it("names the workspace rather than claiming the whole instance is quiet (design-03#D2)", () => {
    render(<NeedsYouInboxEmptyState hiddenCount={0} />);

    expect(screen.getByText("Nothing is waiting on you in Kegmil V2.")).not.toBeNull();
  });

  it("falls back to a workspace-less sentence when the active workspace is unresolved", () => {
    workspaceItems = [];
    render(<NeedsYouInboxEmptyState hiddenCount={0} />);

    expect(screen.getByText("Nothing is waiting on you in this workspace.")).not.toBeNull();
  });

  it("does not congratulate: an empty queue is a normal state, not an achievement (design-03#D2)", () => {
    const { container } = render(<NeedsYouInboxEmptyState hiddenCount={0} />);

    expect(container.querySelector("svg")).toBeNull();
    expect(container.textContent).not.toContain("!");
    expect(container.textContent).not.toContain("caught up");
  });

  it("names all four things it does not count (AC .20)", () => {
    render(<NeedsYouInboxEmptyState hiddenCount={0} />);

    const description = screen.getByText(/does not count bundles in other workspaces/);
    expect(description.textContent).toContain("bundles in other workspaces");
    expect(description.textContent).toContain("pending permission requests");
    expect(description.textContent).toContain("parent-question records");
    expect(description.textContent).toContain("snoozed or dismissed rows");
  });

  it("does not imply anything is hidden when nothing is (AC .20)", () => {
    render(<NeedsYouInboxEmptyState hiddenCount={0} />);

    expect(screen.queryByTestId("needs-you-inbox-hidden-panel")).toBeNull();
  });

  it("discloses and offers to restore hidden bundles when the operator has some hidden", () => {
    render(<NeedsYouInboxEmptyState hiddenCount={2} />);

    expect(screen.getByTestId("needs-you-inbox-hidden-panel")).not.toBeNull();
  });
});

// AC-UI-INBOX-FAILED-001.4: this is the single permitted delta to the
// Needs-you empty state; every case above (workspace naming, no
// congratulation, hidden-bundle disclosure) renders identically when the new
// props are omitted, which is this suite's own before/after proof.
describe("NeedsYouInboxEmptyState — AC-UI-INBOX-FAILED-001.23 failed-tasks clause", () => {
  it("omits the clause entirely when the failed count is not yet known", () => {
    render(<NeedsYouInboxEmptyState hiddenCount={0} />);
    expect(screen.queryByText(/Failed tab/)).toBeNull();
  });

  it("omits the clause when the last failed read failed", () => {
    render(<NeedsYouInboxEmptyState hiddenCount={0} failedCountKnown={false} failedCount={3} />);
    expect(screen.queryByText(/Failed tab/)).toBeNull();
  });

  it("says a failed task exists rather than implying the workspace is quiet, on first render", () => {
    render(<NeedsYouInboxEmptyState hiddenCount={0} failedCountKnown={true} failedCount={2} />);
    expect(
      screen.getByText(
        "This list does not count failed tasks. 2 tasks have also failed. See the Failed tab.",
      ),
    ).not.toBeNull();
  });

  it("does not imply any failed task exists when the workspace holds none", () => {
    render(<NeedsYouInboxEmptyState hiddenCount={0} failedCountKnown={true} failedCount={0} />);
    expect(
      screen.getByText(
        "This list does not count failed tasks. None have failed in this workspace right now; check the Failed tab any time.",
      ),
    ).not.toBeNull();
  });
});
