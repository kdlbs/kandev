import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ClarificationInboxHiddenBundle } from "@/lib/types/clarification-inbox";

const mocks = vi.hoisted(() => ({
  listHidden: vi.fn(),
  restore: vi.fn(),
  bumpRefreshTick: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("@/lib/api/domains/clarification-inbox-api", () => ({
  listHiddenClarificationInbox: (...args: unknown[]) => mocks.listHidden(...args),
  restoreClarificationInboxBundle: (...args: unknown[]) => mocks.restore(...args),
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: Object.assign(vi.fn(), { error: (...args: unknown[]) => mocks.toastError(...args) }),
}));

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (s: any) => unknown) =>
    selector({
      workspaces: { activeId: "w1" },
      bumpNeedsYouInboxRefreshTick: mocks.bumpRefreshTick,
    }),
}));

import { NeedsYouInboxHiddenPanel } from "./needs-you-inbox-hidden-panel";

const SHOW_HIDDEN = "Show hidden";

function hiddenBundle(
  overrides: Partial<ClarificationInboxHiddenBundle> = {},
): ClarificationInboxHiddenBundle {
  return {
    pending_id: "p1",
    task_id: "t1",
    session_id: "s1",
    session_state: "WAITING_FOR_INPUT",
    task_title: "Task",
    created_at: "2026-09-03T04:54:02Z",
    context: "Deploying",
    messages: [],
    state: "dismissed",
    snooze_until: null,
    ...overrides,
  };
}

beforeEach(() => {
  mocks.listHidden.mockReset();
  mocks.restore.mockReset().mockResolvedValue(undefined);
  mocks.bumpRefreshTick.mockReset();
  mocks.toastError.mockReset();
});

afterEach(() => cleanup());

describe("NeedsYouInboxHiddenPanel", () => {
  it("discloses the hidden count without fetching until expanded", () => {
    render(<NeedsYouInboxHiddenPanel hiddenCount={3} />);

    expect(
      screen.getByText("3 questions are hidden by your own dismiss or snooze."),
    ).not.toBeNull();
    expect(mocks.listHidden).not.toHaveBeenCalled();
  });

  it("fetches and lists hidden bundles when expanded", async () => {
    mocks.listHidden.mockResolvedValue({ bundles: [hiddenBundle()], count: 1, total: 1 });
    render(<NeedsYouInboxHiddenPanel hiddenCount={1} />);

    fireEvent.click(screen.getByText(SHOW_HIDDEN));
    expect(await screen.findByTestId("needs-you-inbox-hidden-row")).not.toBeNull();
    expect(mocks.listHidden).toHaveBeenCalledWith("w1");
  });

  it("shows an empty message when nothing is currently hidden despite a stale count", async () => {
    mocks.listHidden.mockResolvedValue({ bundles: [], count: 0, total: 0 });
    render(<NeedsYouInboxHiddenPanel hiddenCount={1} />);

    fireEvent.click(screen.getByText(SHOW_HIDDEN));
    expect(await screen.findByText("Nothing is currently hidden.")).not.toBeNull();
  });

  it("restores a hidden bundle and re-reads the main list (AC .33)", async () => {
    mocks.listHidden.mockResolvedValue({ bundles: [hiddenBundle()], count: 1, total: 1 });
    render(<NeedsYouInboxHiddenPanel hiddenCount={1} />);

    fireEvent.click(screen.getByText(SHOW_HIDDEN));
    fireEvent.click(await screen.findByText("Restore"));

    await act(async () => {
      await Promise.resolve();
    });
    expect(mocks.restore).toHaveBeenCalledWith("p1");
    expect(mocks.bumpRefreshTick).toHaveBeenCalledTimes(1);
  });

  it("shows a retryable error when the hidden read fails", async () => {
    mocks.listHidden.mockRejectedValue(new Error("boom"));
    render(<NeedsYouInboxHiddenPanel hiddenCount={1} />);

    fireEvent.click(screen.getByText(SHOW_HIDDEN));
    expect(await screen.findByText("Could not load hidden questions. Try again.")).not.toBeNull();
  });
});
