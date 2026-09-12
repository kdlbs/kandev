import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api/domains/clarification-inbox-api", () => ({
  listHiddenClarificationInbox: vi.fn().mockResolvedValue({ bundles: [], count: 0, total: 0 }),
  restoreClarificationInboxBundle: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (s: any) => unknown) =>
    selector({ workspaces: { activeId: "w1" }, bumpNeedsYouInboxRefreshTick: vi.fn() }),
}));

import { NeedsYouInboxEmptyState } from "./needs-you-inbox-empty-state";

afterEach(() => cleanup());

describe("NeedsYouInboxEmptyState", () => {
  it("names what it does not count", () => {
    render(<NeedsYouInboxEmptyState hiddenCount={0} />);

    expect(screen.getByText(/does not count bundles in other workspaces/)).not.toBeNull();
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
