import { act, cleanup, render, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useCopilotStore } from "@/hooks/domains/coordinator/copilot-store";
import type { Coordinator, ConversationResponse } from "@/lib/api/domains/coordinator-api";
import { getChatDraftText, setChatDraftText } from "@/lib/local-storage";

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

function openSequenceMock(
  state: { kind: string; [key: string]: unknown } = { kind: "idle" },
  overrides: {
    settledAttemptId?: number | null;
    open?: () => number | null;
    retry?: () => number | null;
  } = {},
) {
  return {
    state,
    settledAttemptId: overrides.settledAttemptId ?? null,
    open: overrides.open ?? vi.fn(() => null),
    retry: overrides.retry ?? vi.fn(() => null),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  useCopilotStore.setState({
    coordinatorId: null,
    open: false,
    chip: null,
    draft: "",
    draftsSwept: false,
  });
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
      openSequenceMock({ kind: "ready", session: conversation }, { open: vi.fn(() => 1) }),
    );
    const { result, rerender } = renderHook(() =>
      useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true),
    );
    expect(result.current.routeSession).toEqual(conversation);

    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    rerender();
    expect(result.current.pendingDraft).toBe(WHY_KAN_1);

    // The ask's own chip-driven reopen (attempt 1) resolves back to the
    // same still-live session (a new object, same id) before Retry ever
    // enters the picture.
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: { ...conversation } }, { settledAttemptId: 1 }),
    );
    rerender();
    expect(result.current.pendingDraft).toBe(WHY_KAN_1);

    // `onRetry` is wired directly to `openSequence.retry` (never touching
    // `entry.chip`/`entry.open`), so this replacement (its own attempt 2)
    // is independent of any chip-driven reopen and must still clear the
    // now-stale draft.
    const retried: ConversationResponse = {
      task_id: "task-2",
      session_id: "session-2",
      archive_state: false,
    };
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: retried }, { settledAttemptId: 2 }),
    );
    rerender();

    expect(result.current.routeSession).toEqual(retried);
    expect(result.current.pendingDraft).toBeUndefined();
  });

  // Review round 3 Finding C: clicking "Ask about this" while the held
  // session is already terminal both seeds pendingDraft (the chip-seed
  // effect) AND re-runs the open sequence (the chip/open-driven effect),
  // which — because the held session is terminal — resolves to a brand new
  // session_id, not the old one. That resolution must not be treated the
  // same as an unrelated Retry: the draft was only ever seeded for this
  // exact open (attempt 1), so it must survive.
  it("keeps a freshly chip-seeded draft when Ask about this itself triggers the session replacement", () => {
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: conversation }, { open: vi.fn(() => 1) }),
    );
    const { result, rerender } = renderHook(() =>
      useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true),
    );
    expect(result.current.routeSession).toEqual(conversation);

    const WHY_KAN_9 = "Why is KAN-9 here?";
    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-9", WHY_KAN_9));
    rerender();
    expect(result.current.pendingDraft).toBe(WHY_KAN_9);

    const replacement: ConversationResponse = {
      task_id: "task-3",
      session_id: "session-3",
      archive_state: false,
    };
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: replacement }, { settledAttemptId: 1 }),
    );
    rerender();

    expect(result.current.routeSession).toEqual(replacement);
    expect(result.current.pendingDraft).toBe(WHY_KAN_9);
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

// Review round 4 Finding D: the self-initiated-open attribution used to rest
// on an optimistic boolean ref, set before `open()`'s outcome (join vs. new
// attempt) was known and never scoped to an actual chip-seeded ask. These
// three tests each reproduce one variant against the fixed attempt-id-based
// attribution in `useReadySessionForCoordinator`.
describe("useCoordinatorCopilot - self-initiated attribution (Finding D)", () => {
  // Review round 4 Finding D1: `entry.chip` also changes identity on a
  // plain chip *removal* (store sets `chip: null` on a new entry object),
  // not just on a genuine new ask. The chip/open effect still fires and
  // still calls `open()` to refresh profile statuses, but with nothing to
  // protect a draft for, that attempt must not be tracked as
  // self-initiated — an unrelated session replacement that later resolves
  // must still clear the (now stale, already-consumed) draft.
  it("clears a stale draft when a plain chip removal is followed by an unrelated session replacement", () => {
    let attemptCounter = 0;
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock(
        { kind: "ready", session: conversation },
        { open: vi.fn(() => ++attemptCounter) },
      ),
    );
    const { result, rerender } = renderHook(() =>
      useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true),
    );
    expect(result.current.routeSession).toEqual(conversation);

    // Attempt 1: the genuine ask.
    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    rerender();
    expect(result.current.pendingDraft).toBe(WHY_KAN_1);

    // Attempt 2: the chip is removed, not re-asked. The effect still fires
    // and still calls open() (profile-status refresh), but must not claim
    // this attempt as self-initiated.
    act(() => result.current.removeChip());
    rerender();

    const unrelatedReplacement: ConversationResponse = {
      task_id: "task-9",
      session_id: "session-9",
      archive_state: false,
    };
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: unrelatedReplacement }, { settledAttemptId: 2 }),
    );
    rerender();

    expect(result.current.pendingDraft).toBeUndefined();
  });

  // Review round 4 Finding D2 (codex): the chip-driven open() call can join
  // an already in-flight sequence (e.g. an independent Retry) instead of
  // starting one of its own — the real `useCopilotOpenSequence.open()`
  // returns `null` in that case. That no-op must not be tracked as
  // self-initiated: the Retry it joined settles under its own, different
  // attempt id and must still clear the draft.
  it("clears the draft when the chip-driven open no-ops into an in-flight Retry", () => {
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: conversation }, { open: vi.fn(() => null) }),
    );
    const { result, rerender } = renderHook(() =>
      useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true),
    );
    expect(result.current.routeSession).toEqual(conversation);

    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    rerender();
    expect(result.current.pendingDraft).toBe(WHY_KAN_1);

    const retried: ConversationResponse = {
      task_id: "task-retry",
      session_id: "session-retry",
      archive_state: false,
    };
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: retried }, { settledAttemptId: 5 }),
    );
    rerender();

    expect(result.current.pendingDraft).toBeUndefined();
  });

  // Review round 4 Finding D3 (kandev-code-reviewer): a self-initiated
  // attempt that resolves to an error must not leak forward and get
  // mistaken for a later, functionally unrelated Retry's own resolution.
  it("does not let a self-initiated open's error leak into suppressing a later unrelated Retry's clear", () => {
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: conversation }, { open: vi.fn(() => 1) }),
    );
    const { result, rerender } = renderHook(() =>
      useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true),
    );
    expect(result.current.routeSession).toEqual(conversation);

    act(() => useCopilotStore.getState().askAboutThis(COORDINATOR_ID, "KAN-1", WHY_KAN_1));
    rerender();
    expect(result.current.pendingDraft).toBe(WHY_KAN_1);

    // The self-initiated attempt (1) fails outright.
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "error", error: "open-failed" }, { settledAttemptId: 1 }),
    );
    rerender();

    // A later, unrelated Retry succeeds under its own attempt id (2) into a
    // genuinely different session.
    const retried: ConversationResponse = {
      task_id: "task-retry-2",
      session_id: "session-retry-2",
      archive_state: false,
    };
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: retried }, { settledAttemptId: 2 }),
    );
    rerender();

    expect(result.current.pendingDraft).toBeUndefined();
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

describe("useCoordinatorCopilot - stored composer draft sweep", () => {
  beforeEach(() => {
    mocks.useCopilotOpenSequence.mockReturnValue(
      openSequenceMock({ kind: "ready", session: conversation }),
    );
    setChatDraftText(conversation.session_id, "left over from a reset slot");
  });

  afterEach(() => setChatDraftText(conversation.session_id, ""));

  it("clears the stored draft once when the ready session first appears for a slot", () => {
    act(() => useCopilotStore.getState().setOpen(COORDINATOR_ID, true));
    renderHook(() => useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true));

    expect(getChatDraftText(conversation.session_id)).toBe("");
    expect(useCopilotStore.getState().draftsSwept).toBe(true);
  });

  it("hands the composer a loading state until the sweep has run, then the ready session", () => {
    const observed: string[] = [];
    act(() => useCopilotStore.getState().setOpen(COORDINATOR_ID, true));
    const { result } = renderHook(() => {
      const value = useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true);
      observed.push(
        `${value.openSequence.state.kind}:${getChatDraftText(conversation.session_id)}`,
      );
      return value;
    });

    expect(observed[0]).toBe("loading:left over from a reset slot");
    expect(result.current.openSequence.state.kind).toBe("ready");
    expect(getChatDraftText(conversation.session_id)).toBe("");
  });

  it("keeps text typed after the sweep across a close and reopen of the same slot", () => {
    act(() => useCopilotStore.getState().setOpen(COORDINATOR_ID, true));
    const { result } = renderHook(() => useCoordinatorCopilot(WORKSPACE_ID, COORDINATOR_ID, true));
    setChatDraftText(conversation.session_id, "typed after");

    act(() => result.current.handleOpenChange(false));
    act(() => result.current.handleOpenChange(true));

    expect(getChatDraftText(conversation.session_id)).toBe("typed after");
  });
});
