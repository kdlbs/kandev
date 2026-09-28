import { act, cleanup, render, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useCopilotStore } from "@/hooks/domains/coordinator/copilot-store";
import type { Coordinator, ConversationResponse } from "@/lib/api/domains/coordinator-api";

const mocks = vi.hoisted(() => ({
  useFeature: vi.fn(),
  useCoordinatorLauncher: vi.fn(),
  useCopilotOpenSequence: vi.fn(),
  getCoordinator: vi.fn(),
  openConversation: vi.fn(),
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
vi.mock("@/lib/api/domains/coordinator-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/coordinator-api")>(
    "@/lib/api/domains/coordinator-api",
  );
  return {
    ...actual,
    getCoordinator: mocks.getCoordinator,
    openConversation: mocks.openConversation,
  };
});

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

// Review round 2 Finding B: a chip-seeded draft that was already sent as a
// message must not resurrect itself into the composer of the fresh session
// Retry produces after the prior one ended. The hook cannot see "was it sent"
// directly, but it does see the session identity change Retry always
// produces (a new session_id replaces the ended one), which is exactly the
// boundary where a carried-over seed would otherwise leak into a session it
// was never meant for.
describe("useCoordinatorCopilot - pendingDraft clearing on session replacement", () => {
  it("clears pendingDraft when Retry replaces the session with a new one", () => {
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: conversation }),
    );
    const { result, rerender } = renderHook(() =>
      useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true),
    );
    expect(result.current.routeSession).toEqual(conversation);

    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    rerender();
    expect(result.current.pendingDraft).toBe(WHY_KAN_1);

    const retried: ConversationResponse = {
      task_id: "task-2",
      session_id: "session-2",
      archive_state: false,
    };
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: retried }),
    );
    rerender();

    expect(result.current.routeSession).toEqual(retried);
    expect(result.current.pendingDraft).toBeUndefined();
  });

  it("keeps pendingDraft when the ready session's id is unchanged (same session, remounted via askKey)", () => {
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: conversation }),
    );
    const { result, rerender } = renderHook(() =>
      useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true),
    );

    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    rerender();
    expect(result.current.pendingDraft).toBe(WHY_KAN_1);

    // Re-deliver the identical "ready" session (e.g. a re-render from an
    // unrelated state change, or reopening a still-live conversation).
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: { ...conversation } }),
    );
    rerender();

    expect(result.current.pendingDraft).toBe(WHY_KAN_1);
  });
});

/** The mount site keys the controller on `coordinatorId`
 * (`coordinator-route-content.tsx`), so a coordinator switch always
 * unmounts and remounts this hook rather than handing it a changed prop.
 * This exercises that real remount, composed with the real
 * `useCopilotOpenSequence`, instead of mocking the class of bug away. */
describe("useCoordinatorCopilot - keyed remount across a coordinator switch", () => {
  function coordinator(id: string): Coordinator {
    return {
      id,
      workspace_id: WORKSPACE_ID,
      name: `Coordinator ${id}`,
      agent_profile_id: "agent-1",
      executor_profile_id: "executor-1",
      context: "",
      conversation_task_id: null,
      created_at: "2026-09-28T00:00:00Z",
      updated_at: "2026-09-28T00:00:00Z",
      agent_profile_status: "ok",
      executor_profile_status: "ok",
    };
  }

  function sessionFor(id: string): ConversationResponse {
    return { task_id: `task-${id}`, session_id: `session-${id}`, archive_state: false };
  }

  function Harness({
    coordinatorId,
    onResult,
  }: {
    coordinatorId: string;
    onResult: (result: ReturnType<typeof useCoordinatorCopilot>) => void;
  }) {
    onResult(useCoordinatorCopilot(WORKSPACE_ID, coordinatorId, true));
    return null;
  }

  it("never leaks the previous coordinator's session or draft into the newly-viewed one", async () => {
    const actual = await vi.importActual<
      typeof import("@/hooks/domains/coordinator/use-copilot-open-sequence")
    >("@/hooks/domains/coordinator/use-copilot-open-sequence");
    mocks.useCopilotOpenSequence.mockImplementation(actual.useCopilotOpenSequence);
    mocks.getCoordinator.mockImplementation(async (_workspaceId: string, id: string) =>
      coordinator(id),
    );
    mocks.openConversation.mockImplementation(async (_workspaceId: string, id: string) =>
      sessionFor(id),
    );

    let latest: ReturnType<typeof useCoordinatorCopilot> | undefined;
    const onResult = (result: ReturnType<typeof useCoordinatorCopilot>) => {
      latest = result;
    };

    // Ready A -> unopened B: A opens and reaches ready; B has never been
    // opened, so the switch must show no session at all, never A's.
    act(() => useCopilotStore.getState().setOpen("coord-a", true));
    const { rerender } = render(
      <Harness key="coord-a" coordinatorId="coord-a" onResult={onResult} />,
    );
    await waitFor(() => expect(latest?.routeSession).toEqual(sessionFor("coord-a")));

    rerender(<Harness key="coord-b" coordinatorId="coord-b" onResult={onResult} />);
    expect(latest?.routeSession).toBeNull();
    expect(latest?.pendingDraft).toBeUndefined();

    // Open A -> open B (no chip): B is also marked open in the store, so
    // the remounted controller opens B's own conversation, not A's.
    act(() => useCopilotStore.getState().setOpen("coord-b", true));
    await waitFor(() => expect(latest?.routeSession).toEqual(sessionFor("coord-b")));
    expect(latest?.routeSession).not.toEqual(sessionFor("coord-a"));

    // A delayed A response lands after the switch: re-open A, hold its GET
    // in flight, switch away to C before it resolves, then let A's response
    // land late. C's own session must be unaffected.
    let resolveAGet: ((value: Coordinator) => void) | undefined;
    mocks.getCoordinator.mockImplementation((_workspaceId: string, id: string) => {
      if (id === "coord-a") {
        return new Promise<Coordinator>((resolve) => {
          resolveAGet = resolve;
        });
      }
      return Promise.resolve(coordinator(id));
    });
    act(() => useCopilotStore.getState().setOpen("coord-a", true));
    rerender(<Harness key="coord-a" coordinatorId="coord-a" onResult={onResult} />);
    await waitFor(() => expect(resolveAGet).toBeDefined());

    act(() => useCopilotStore.getState().setOpen("coord-c", true));
    rerender(<Harness key="coord-c" coordinatorId="coord-c" onResult={onResult} />);
    await waitFor(() => expect(latest?.routeSession).toEqual(sessionFor("coord-c")));

    resolveAGet?.(coordinator("coord-a"));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(latest?.routeSession).toEqual(sessionFor("coord-c"));
  });
});
