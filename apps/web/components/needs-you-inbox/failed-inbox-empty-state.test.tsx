import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

let workspaceItems: Array<{ id: string; name: string }> = [{ id: "w1", name: "Kegmil V2" }];

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (s: any) => unknown) =>
    selector({ workspaces: { activeId: "w1", items: workspaceItems } }),
}));

import { FailedInboxEmptyState } from "./failed-inbox-empty-state";

afterEach(() => {
  cleanup();
  workspaceItems = [{ id: "w1", name: "Kegmil V2" }];
});

describe("FailedInboxEmptyState", () => {
  it("names the active workspace rather than the whole instance (AC .21)", () => {
    render(<FailedInboxEmptyState />);
    expect(screen.getByText("Nothing has failed in Kegmil V2.")).not.toBeNull();
  });

  it("names no workspace when the active workspace cannot be resolved (AC .21)", () => {
    workspaceItems = [];
    render(<FailedInboxEmptyState />);
    expect(screen.getByText("Nothing has failed in this workspace.")).not.toBeNull();
  });

  it("does not congratulate the operator (AC .21)", () => {
    const { container } = render(<FailedInboxEmptyState />);
    expect(container.textContent).not.toContain("!");
    expect(container.textContent).not.toContain("caught up");
  });
});
