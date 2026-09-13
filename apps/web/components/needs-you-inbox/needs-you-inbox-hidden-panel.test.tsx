import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ClarificationInboxHiddenBundle } from "@/lib/types/clarification-inbox";

const mocks = vi.hoisted(() => ({
  listHidden: vi.fn(),
  restore: vi.fn(),
  bumpRefreshTick: vi.fn(),
  toastError: vi.fn(),
  activeWorkspaceId: "w1",
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
      workspaces: { activeId: mocks.activeWorkspaceId },
      bumpNeedsYouInboxRefreshTick: mocks.bumpRefreshTick,
    }),
}));

import { NeedsYouInboxHiddenPanel } from "./needs-you-inbox-hidden-panel";

const SHOW_HIDDEN = "Show hidden";
const WORKSPACE_ONE_CONTEXT = "From workspace one";
const WORKSPACE_TWO_CONTEXT = "From workspace two";

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
  mocks.activeWorkspaceId = "w1";
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

  it("prefers the fetched total over a stale hiddenCount prop once loaded", async () => {
    mocks.listHidden.mockResolvedValue({ bundles: [hiddenBundle()], count: 1, total: 5 });
    render(<NeedsYouInboxHiddenPanel hiddenCount={1} />);

    expect(screen.getByText("1 question is hidden by your own dismiss or snooze.")).not.toBeNull();

    fireEvent.click(screen.getByText(SHOW_HIDDEN));
    await screen.findByTestId("needs-you-inbox-hidden-row");

    expect(
      screen.getByText("5 questions are hidden by your own dismiss or snooze."),
    ).not.toBeNull();
  });

  it("clears the stale list and re-fetches when the active workspace changes", async () => {
    mocks.listHidden.mockResolvedValue({
      bundles: [hiddenBundle({ pending_id: "w1-p1", context: WORKSPACE_ONE_CONTEXT })],
      count: 1,
      total: 1,
    });
    const { rerender } = render(<NeedsYouInboxHiddenPanel hiddenCount={1} />);

    fireEvent.click(screen.getByText(SHOW_HIDDEN));
    await screen.findByText(WORKSPACE_ONE_CONTEXT);
    expect(mocks.listHidden).toHaveBeenCalledWith("w1");

    mocks.activeWorkspaceId = "w2";
    mocks.listHidden.mockResolvedValue({
      bundles: [hiddenBundle({ pending_id: "w2-p1", context: WORKSPACE_TWO_CONTEXT })],
      count: 1,
      total: 1,
    });
    rerender(<NeedsYouInboxHiddenPanel hiddenCount={1} />);

    expect(await screen.findByText(WORKSPACE_TWO_CONTEXT)).not.toBeNull();
    expect(screen.queryByText(WORKSPACE_ONE_CONTEXT)).toBeNull();
    expect(mocks.listHidden).toHaveBeenLastCalledWith("w2");
  });

  it("discards a stale in-flight response from the previous workspace", async () => {
    let resolveFirst: ((value: unknown) => void) | undefined;
    mocks.listHidden.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve;
        }),
    );
    const { rerender } = render(<NeedsYouInboxHiddenPanel hiddenCount={1} />);
    fireEvent.click(screen.getByText(SHOW_HIDDEN));

    mocks.activeWorkspaceId = "w2";
    mocks.listHidden.mockResolvedValue({
      bundles: [hiddenBundle({ pending_id: "w2-p1", context: WORKSPACE_TWO_CONTEXT })],
      count: 1,
      total: 1,
    });
    rerender(<NeedsYouInboxHiddenPanel hiddenCount={1} />);
    await screen.findByText(WORKSPACE_TWO_CONTEXT);

    await act(async () => {
      resolveFirst?.({
        bundles: [hiddenBundle({ pending_id: "w1-p1", context: WORKSPACE_ONE_CONTEXT })],
        count: 1,
        total: 1,
      });
      await Promise.resolve();
    });

    expect(screen.queryByText(WORKSPACE_ONE_CONTEXT)).toBeNull();
    expect(screen.getByText(WORKSPACE_TWO_CONTEXT)).not.toBeNull();
  });
});
