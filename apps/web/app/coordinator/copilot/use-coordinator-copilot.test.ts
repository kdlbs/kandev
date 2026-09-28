import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useCopilotStore } from "@/hooks/domains/coordinator/copilot-store";
import type { ConversationResponse } from "@/lib/api/domains/coordinator-api";

const mocks = vi.hoisted(() => ({
  useFeature: vi.fn(),
  useCoordinatorLauncher: vi.fn(),
  useCopilotOpenSequence: vi.fn(),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: mocks.useFeature,
}));
vi.mock("@/hooks/domains/coordinator/use-coordinator-launcher", () => ({
  useCoordinatorLauncher: mocks.useCoordinatorLauncher,
}));
vi.mock("@/hooks/domains/coordinator/use-copilot-open-sequence", () => ({
  useCopilotOpenSequence: mocks.useCopilotOpenSequence,
}));

import { useCoordinatorCopilot } from "./use-coordinator-copilot";

const WORKSPACE_ID = "ws-1";
const COORDINATOR_ID = "coord-1";

const conversation: ConversationResponse = {
  task_id: "task-1",
  session_id: "session-1",
  archive_state: false,
};

const WHY_KAN_1 = "Why is KAN-1 here?";

function openSequenceMock(state: { kind: string; [key: string]: unknown } = { kind: "idle" }) {
  return { state, open: vi.fn(), retry: vi.fn() };
}

beforeEach(() => {
  vi.clearAllMocks();
  useCopilotStore.setState({ entries: {} });
  mocks.useFeature.mockReturnValue(true);
  mocks.useCoordinatorLauncher.mockReturnValue({
    coordinator: null,
    loading: false,
    busy: false,
    gone: false,
  });
  mocks.useCopilotOpenSequence.mockReturnValue(openSequenceMock());
});

afterEach(cleanup);

describe("useCoordinatorCopilot", () => {
  it("is disabled when the feature flag is off", () => {
    mocks.useFeature.mockReturnValue(false);
    const { result } = renderHook(() => useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true));
    expect(result.current.enabled).toBe(false);
    expect(mocks.useCoordinatorLauncher).toHaveBeenCalledWith(WORKSPACE_ID, null, null);
    expect(mocks.useCopilotOpenSequence).toHaveBeenCalledWith(WORKSPACE_ID, null);
  });

  it("is disabled for a reader even with the flag on", () => {
    const { result } = renderHook(() => useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, false));
    expect(result.current.enabled).toBe(false);
  });

  it("is enabled for a manager with the flag on", () => {
    const { result } = renderHook(() => useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true));
    expect(result.current.enabled).toBe(true);
    expect(mocks.useCoordinatorLauncher).toHaveBeenCalledWith(WORKSPACE_ID, COORDINATOR_ID, null);
  });

  it("opening the popover sets the store entry and runs the open sequence", () => {
    const sequence = openSequenceMock();
    mocks.useCopilotOpenSequence.mockReturnValue(sequence);
    const { result } = renderHook(() => useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true));

    act(() => result.current.handleOpenChange(true));

    expect(useCopilotStore.getState().getEntry(COORDINATOR_ID).open).toBe(true);
    expect(sequence.open).toHaveBeenCalledTimes(1);
  });

  it("a second Ask about this while already open re-runs the open sequence", () => {
    const sequence = openSequenceMock();
    mocks.useCopilotOpenSequence.mockReturnValue(sequence);
    renderHook(() => useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true));

    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    expect(sequence.open).toHaveBeenCalledTimes(1);

    act(() =>
      useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-2", "Why is KAN-2 here?"),
    );
    expect(sequence.open).toHaveBeenCalledTimes(2);
  });

  it("stores the ready session and stops the launcher from calling the route", () => {
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: conversation }),
    );
    const { result } = renderHook(() => useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true));
    expect(result.current.routeSession).toEqual(conversation);
  });
});

describe("useCoordinatorCopilot - closing and gone states", () => {
  it("a gone state clears the chip and draft but keeps open", () => {
    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    mocks.useCopilotOpenSequence.mockReturnValue(openSequenceMock({ kind: "gone" }));

    renderHook(() => useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true));

    const entry = useCopilotStore.getState().getEntry(COORDINATOR_ID);
    expect(entry.chip).toBeNull();
    expect(entry.draft).toBe("");
    expect(entry.open).toBe(true);
  });

  it("closing a gone popover removes the entry entirely", () => {
    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    mocks.useCopilotOpenSequence.mockReturnValue(openSequenceMock({ kind: "gone" }));
    const { result } = renderHook(() => useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true));

    act(() => result.current.handleOpenChange(false));

    expect(useCopilotStore.getState().getEntry(COORDINATOR_ID)).toEqual({
      open: false,
      chip: null,
      draft: "",
    });
  });

  it("closing a normal popover keeps the chip and draft", () => {
    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    const { result } = renderHook(() => useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true));

    act(() => result.current.handleOpenChange(false));

    const entry = useCopilotStore.getState().getEntry(COORDINATOR_ID);
    expect(entry.open).toBe(false);
    expect(entry.chip).toEqual({ id: "KAN-1", label: "KAN-1" });
  });

  it("a launcher 404 while closed removes the entry", () => {
    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    act(() => useCopilotStore.getState().setOpen(COORDINATOR_ID, false));
    mocks.useCoordinatorLauncher.mockReturnValue({
      coordinator: null,
      loading: false,
      busy: false,
      gone: true,
    });

    renderHook(() => useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true));

    expect(useCopilotStore.getState().getEntry(COORDINATOR_ID)).toEqual({
      open: false,
      chip: null,
      draft: "",
    });
  });
});

describe("useCoordinatorCopilot - draft, askKey and coordinator switching", () => {
  it("seeds pendingDraft and bumps askKey on a new ask, clearing the store draft", () => {
    const { result, rerender } = renderHook(() =>
      useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true),
    );
    const initialAskKey = result.current.askKey;

    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    rerender();

    expect(result.current.pendingDraft).toBe(WHY_KAN_1);
    expect(result.current.askKey).toBe(initialAskKey + 1);
    expect(useCopilotStore.getState().getEntry(COORDINATOR_ID).draft).toBe("");
  });

  it("a same-item re-ask bumps askKey again with the same question", () => {
    const { result, rerender } = renderHook(() =>
      useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true),
    );
    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    rerender();
    const firstAskKey = result.current.askKey;

    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    rerender();

    expect(result.current.askKey).toBe(firstAskKey + 1);
    expect(result.current.pendingDraft).toBe(WHY_KAN_1);
  });

  it("suggest seeds pendingDraft and bumps askKey without touching the store", () => {
    const { result, rerender } = renderHook(() =>
      useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true),
    );
    const initialAskKey = result.current.askKey;

    act(() => result.current.suggest("What needs me first, and why?"));
    rerender();

    expect(result.current.pendingDraft).toBe("What needs me first, and why?");
    expect(result.current.askKey).toBe(initialAskKey + 1);
    expect(useCopilotStore.getState().getEntry(COORDINATOR_ID)).toEqual({
      open: false,
      chip: null,
      draft: "",
    });
  });

  it("removeChip clears only the chip in the store", () => {
    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    const { result } = renderHook(() => useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true));

    act(() => result.current.removeChip());

    const entry = useCopilotStore.getState().getEntry(COORDINATOR_ID);
    expect(entry.chip).toBeNull();
  });

  it("resets routeSession and pendingDraft when the viewed coordinator changes", () => {
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: conversation }),
    );
    const { result, rerender } = renderHook(
      ({ coordinatorId }: { coordinatorId: string }) =>
        useCoordinatorCopilot(WORKSPACE_ID, coordinatorId, true),
      { initialProps: { coordinatorId: "coord-1" } },
    );
    expect(result.current.routeSession).toEqual(conversation);

    mocks.useCopilotOpenSequence.mockReturnValue(openSequenceMock({ kind: "idle" }));
    rerender({ coordinatorId: "coord-2" });

    expect(result.current.routeSession).toBeNull();
    expect(result.current.pendingDraft).toBeUndefined();
  });

  it("clears pendingDraft on close so reopening never reinserts the already-applied draft", () => {
    const { result, rerender } = renderHook(() =>
      useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true),
    );

    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    rerender();
    expect(result.current.pendingDraft).toBe(WHY_KAN_1);

    act(() => result.current.handleOpenChange(false));
    rerender();

    expect(result.current.pendingDraft).toBeUndefined();
  });

  it("never derives the launcher busy-state from a previous coordinator's stale session on a coordinator switch", () => {
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: conversation }),
    );
    const { rerender } = renderHook(
      ({ coordinatorId }: { coordinatorId: string }) =>
        useCoordinatorCopilot(WORKSPACE_ID, coordinatorId, true),
      { initialProps: { coordinatorId: "coord-1" } },
    );

    mocks.useCoordinatorLauncher.mockClear();
    mocks.useCopilotOpenSequence.mockReturnValue(openSequenceMock({ kind: "idle" }));
    rerender({ coordinatorId: "coord-2" });

    for (const call of mocks.useCoordinatorLauncher.mock.calls) {
      expect(call).toEqual([WORKSPACE_ID, "coord-2", null]);
    }
  });
});
