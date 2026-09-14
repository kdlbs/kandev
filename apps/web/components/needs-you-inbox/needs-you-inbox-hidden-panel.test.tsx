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

describe("NeedsYouInboxHiddenPanel staleness guards", () => {
  it("does not let a restore begun before a workspace switch overwrite the new workspace's panel", async () => {
    mocks.listHidden.mockResolvedValueOnce({
      bundles: [hiddenBundle({ pending_id: "w1-p1", context: WORKSPACE_ONE_CONTEXT })],
      count: 1,
      total: 1,
    });
    const { rerender } = render(<NeedsYouInboxHiddenPanel hiddenCount={1} />);
    fireEvent.click(screen.getByText(SHOW_HIDDEN));
    await screen.findByText(WORKSPACE_ONE_CONTEXT);

    let resolveRestore: (() => void) | undefined;
    mocks.restore.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          resolveRestore = resolve;
        }),
    );
    fireEvent.click(screen.getByText("Restore"));

    mocks.activeWorkspaceId = "w2";
    // Both the workspace-change and the hiddenCount-change effects can fire
    // on this rerender; give every call the same answer rather than pinning
    // an exact call count that isn't this test's concern.
    mocks.listHidden.mockResolvedValue({ bundles: [], count: 0, total: 0 });
    rerender(<NeedsYouInboxHiddenPanel hiddenCount={0} />);
    await screen.findByText("Nothing is currently hidden.");

    const callsBeforeRestoreSettles = mocks.listHidden.mock.calls.length;
    await act(async () => {
      resolveRestore?.();
      await Promise.resolve();
      await Promise.resolve();
    });

    // The stale w1 continuation must not re-fetch w1 nor reintroduce its row.
    expect(mocks.listHidden.mock.calls.length).toBe(callsBeforeRestoreSettles);
    expect(screen.getByText("Nothing is currently hidden.")).not.toBeNull();
    expect(screen.queryByText(WORKSPACE_ONE_CONTEXT)).toBeNull();
  });

  it("re-enumerates while open when the main list's hidden count changes elsewhere", async () => {
    mocks.listHidden.mockResolvedValueOnce({
      bundles: [
        hiddenBundle({ pending_id: "p1", context: "First" }),
        hiddenBundle({ pending_id: "p2", context: "Second" }),
      ],
      count: 2,
      total: 2,
    });
    const { rerender } = render(<NeedsYouInboxHiddenPanel hiddenCount={2} />);
    fireEvent.click(screen.getByText(SHOW_HIDDEN));
    await screen.findByText("First");
    await screen.findByText("Second");

    // Another row was dismissed elsewhere: the main list's hiddenCount grew
    // without any action inside this panel.
    mocks.listHidden.mockResolvedValueOnce({
      bundles: [
        hiddenBundle({ pending_id: "p1", context: "First" }),
        hiddenBundle({ pending_id: "p2", context: "Second" }),
        hiddenBundle({ pending_id: "p3", context: "Third" }),
      ],
      count: 3,
      total: 3,
    });
    rerender(<NeedsYouInboxHiddenPanel hiddenCount={3} />);

    expect(await screen.findByText("Third")).not.toBeNull();
    expect(mocks.listHidden).toHaveBeenCalledTimes(2);
    expect(
      screen.getByText("3 questions are hidden by your own dismiss or snooze."),
    ).not.toBeNull();
  });

  it("re-enumerates while open when a newer main-list response keeps the same hidden count", async () => {
    mocks.listHidden.mockResolvedValueOnce({
      bundles: [hiddenBundle({ pending_id: "p1", context: "First" })],
      count: 1,
      total: 1,
    });
    const { rerender } = render(<NeedsYouInboxHiddenPanel hiddenCount={1} listRevision={1} />);
    fireEvent.click(screen.getByText(SHOW_HIDDEN));
    await screen.findByText("First");

    // A newer main-list response replaced one hidden row with another, so the
    // count stayed at one while the disclosed set changed.
    mocks.listHidden.mockResolvedValueOnce({
      bundles: [hiddenBundle({ pending_id: "p2", context: "Second" })],
      count: 1,
      total: 1,
    });
    rerender(<NeedsYouInboxHiddenPanel hiddenCount={1} listRevision={2} />);

    expect(await screen.findByText("Second")).not.toBeNull();
    expect(screen.queryByText("First")).toBeNull();
    expect(mocks.listHidden).toHaveBeenCalledTimes(2);
  });
});
