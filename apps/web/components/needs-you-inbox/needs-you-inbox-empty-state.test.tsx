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
