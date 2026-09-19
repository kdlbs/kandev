import { describe, expect, it } from "vitest";
import { createAppStore } from "./store";
import {
  acknowledgeWorkflowSessionFocus,
  bindWorkflowSessionFocus,
  cancelWorkflowSessionFocus,
  createWorkflowSessionFocusState,
  beginWorkflowSessionFocus,
  reconcileWorkflowSessionFocus,
} from "./workflow-session-focus";

const START = {
  taskId: "task-1",
  workflowId: "workflow-1",
  destinationStepId: "step-implement",
  presentationToken: 7,
  navigationRevision: 3,
};
const SOURCE_SESSION_ID = "session-astra";
const DESTINATION_SESSION_ID = "session-luna";
const NEWER_SESSION_ID = "session-newer";
const LIVE_UPDATED_AT = "2026-09-19T10:00:02Z";
const RESPONSE_UPDATED_AT = "2026-09-19T10:00:01Z";
const FOCUS_INTENT_ERROR = "test focus intent did not start";

const COMMITTED_ROUTE = {
  operation_id: "operation-1",
  destination_step_id: "step-implement",
  entry_identity: "entry:00000000000000000042",
  destination_session_id: DESTINATION_SESSION_ID,
  phase: "committed",
};

function started(): {
  state: ReturnType<typeof createWorkflowSessionFocusState>;
  requestId: number;
} {
  const result = beginWorkflowSessionFocus(createWorkflowSessionFocusState(), START);
  if (result.requestId === null) throw new Error(FOCUS_INTENT_ERROR);
  return { state: result.state, requestId: result.requestId };
}

// eslint-disable-next-line max-lines-per-function -- state-machine scenarios share one transition fixture
describe("workflow session focus state", () => {
  it("applies the resolved recipient atomically in the app store and clears the old pin", () => {
    const store = createAppStore();
    store.setState((state) => {
      state.tasks.activeTaskId = START.taskId;
      state.tasks.activeSessionId = SOURCE_SESSION_ID;
      state.tasks.pinnedSessionId = SOURCE_SESSION_ID;
      state.taskRemoval.navigationRevision = START.navigationRevision;
      state.kanban.tasks = [
        {
          id: START.taskId,
          metadata: { workflow_session_route: COMMITTED_ROUTE },
        },
      ] as never;
      state.taskSessionsByTask.itemsByTaskId[START.taskId] = [
        { id: SOURCE_SESSION_ID },
        { id: DESTINATION_SESSION_ID },
      ] as never;
      state.taskSessions.items[DESTINATION_SESSION_ID] = {
        id: DESTINATION_SESSION_ID,
        task_id: START.taskId,
      } as never;
    });

    const requestId = store.getState().beginWorkflowSessionFocus(START);
    expect(requestId).toBe(1);
    if (requestId === null) throw new Error(FOCUS_INTENT_ERROR);
    store.getState().bindWorkflowSessionFocus({
      requestId,
      presentationToken: START.presentationToken,
      entryIdentity: COMMITTED_ROUTE.entry_identity,
    });
    store.getState().reconcileWorkflowSessionFocus(START.taskId);

    expect(store.getState().tasks.activeSessionId).toBe(DESTINATION_SESSION_ID);
    expect(store.getState().tasks.pinnedSessionId).toBeNull();
    expect(store.getState().tasks.lastSessionByTaskId[START.taskId]).toBe(DESTINATION_SESSION_ID);
    expect(store.getState().workflowSessionFocus.request?.sessionId).toBe(DESTINATION_SESSION_ID);

    store.getState().setActiveSession(START.taskId, SOURCE_SESSION_ID);
    expect(store.getState().workflowSessionFocus.request).toBeNull();
  });

  it("reconsiders the cached live route when a move response is delayed", () => {
    const store = createAppStore();
    store.setState((state) => {
      state.tasks.activeTaskId = START.taskId;
      state.tasks.activeSessionId = SOURCE_SESSION_ID;
      state.taskRemoval.navigationRevision = START.navigationRevision;
      state.kanban.tasks = [
        {
          id: START.taskId,
          updatedAt: LIVE_UPDATED_AT,
          metadata: { workflow_session_route: COMMITTED_ROUTE },
        },
      ] as never;
      state.taskSessionsByTask.itemsByTaskId[START.taskId] = [
        { id: SOURCE_SESSION_ID },
        { id: DESTINATION_SESSION_ID },
      ] as never;
    });

    const requestId = store.getState().beginWorkflowSessionFocus(START);
    if (requestId === null) throw new Error(FOCUS_INTENT_ERROR);
    store.getState().bindWorkflowSessionFocus({
      requestId,
      presentationToken: START.presentationToken,
      entryIdentity: COMMITTED_ROUTE.entry_identity,
    });
    store.getState().reconcileWorkflowSessionFocus(START.taskId, {
      metadata: { workflow_session_route: { ...COMMITTED_ROUTE, phase: "prepared" } },
      updatedAt: RESPONSE_UPDATED_AT,
      entryIdentity: COMMITTED_ROUTE.entry_identity,
    });

    expect(store.getState().tasks.activeSessionId).toBe(DESTINATION_SESSION_ID);
    expect(store.getState().workflowSessionFocus.request?.sessionId).toBe(DESTINATION_SESSION_ID);
  });

  it("keeps an intent pending when its response route is older than a newer live entry", () => {
    const store = createAppStore();
    const newerLiveRoute = {
      ...COMMITTED_ROUTE,
      entry_identity: "entry:00000000000000000043",
      destination_session_id: NEWER_SESSION_ID,
    };
    store.setState((state) => {
      state.tasks.activeTaskId = START.taskId;
      state.tasks.activeSessionId = SOURCE_SESSION_ID;
      state.taskRemoval.navigationRevision = START.navigationRevision;
      state.kanban.tasks = [
        {
          id: START.taskId,
          updatedAt: LIVE_UPDATED_AT,
          metadata: { workflow_session_route: newerLiveRoute },
        },
      ] as never;
      state.taskSessionsByTask.itemsByTaskId[START.taskId] = [
        { id: SOURCE_SESSION_ID },
        { id: DESTINATION_SESSION_ID },
        { id: NEWER_SESSION_ID },
      ] as never;
    });

    const requestId = store.getState().beginWorkflowSessionFocus(START);
    if (requestId === null) throw new Error(FOCUS_INTENT_ERROR);
    store.getState().bindWorkflowSessionFocus({
      requestId,
      presentationToken: START.presentationToken,
      entryIdentity: COMMITTED_ROUTE.entry_identity,
    });
    store.getState().reconcileWorkflowSessionFocus(START.taskId, {
      metadata: { workflow_session_route: COMMITTED_ROUTE },
      updatedAt: RESPONSE_UPDATED_AT,
      entryIdentity: COMMITTED_ROUTE.entry_identity,
    });

    expect(store.getState().tasks.activeSessionId).toBe(SOURCE_SESSION_ID);
    expect(store.getState().workflowSessionFocus.intent).not.toBeNull();
    expect(store.getState().workflowSessionFocus.request).toBeNull();
  });

  it("binds a response entry and resolves it only for the committed task-owned recipient", () => {
    const startedState = started();
    expect(startedState.requestId).toBe(1);

    const bound = bindWorkflowSessionFocus(startedState.state, {
      requestId: startedState.requestId,
      presentationToken: START.presentationToken,
      entryIdentity: COMMITTED_ROUTE.entry_identity,
    });
    const unresolved = reconcileWorkflowSessionFocus(bound, {
      activeTaskId: START.taskId,
      navigationRevision: START.navigationRevision,
      routeMetadata: { workflow_session_route: COMMITTED_ROUTE },
      knownSessionIds: [SOURCE_SESSION_ID],
    });

    expect(unresolved.sessionId).toBeNull();
    expect(unresolved.state.intent).not.toBeNull();

    const resolved = reconcileWorkflowSessionFocus(bound, {
      activeTaskId: START.taskId,
      navigationRevision: START.navigationRevision,
      routeMetadata: { workflow_session_route: COMMITTED_ROUTE },
      knownSessionIds: [SOURCE_SESSION_ID, DESTINATION_SESSION_ID],
    });

    expect(resolved.sessionId).toBe(DESTINATION_SESSION_ID);
    expect(resolved.state.intent).toBeNull();
    expect(resolved.state.request).toEqual({
      requestId: startedState.requestId,
      presentationToken: START.presentationToken,
      taskId: START.taskId,
      sessionId: DESTINATION_SESSION_ID,
    });
  });

  it("rejects prepared, foreign, stale, and mismatched routes", () => {
    const startedState = started();
    const bound = bindWorkflowSessionFocus(startedState.state, {
      requestId: startedState.requestId,
      presentationToken: START.presentationToken,
      entryIdentity: COMMITTED_ROUTE.entry_identity,
    });

    for (const routeMetadata of [
      { workflow_session_route: { ...COMMITTED_ROUTE, phase: "prepared" } },
      { workflow_session_route: { ...COMMITTED_ROUTE, entry_identity: "entry:other" } },
      { workflow_session_route: { ...COMMITTED_ROUTE, destination_step_id: "step-review" } },
      { workflow_session_route: { ...COMMITTED_ROUTE, destination_session_id: "session-other" } },
      { workflow_session_route: { ...COMMITTED_ROUTE, operation_id: "" } },
    ]) {
      const result = reconcileWorkflowSessionFocus(bound, {
        activeTaskId: START.taskId,
        navigationRevision: START.navigationRevision,
        routeMetadata,
        knownSessionIds: [DESTINATION_SESSION_ID],
      });
      expect(result.sessionId).toBeNull();
      expect(result.state.intent).not.toBeNull();
    }
  });

  it("cancels on a later navigation, newer request, and a missing response identity", () => {
    const first = started();
    const canceled = cancelWorkflowSessionFocus(first.state, {
      taskId: START.taskId,
      presentationToken: START.presentationToken,
    });
    expect(canceled.intent).toBeNull();

    const second = beginWorkflowSessionFocus(canceled, { ...START, presentationToken: 8 });
    const newer = beginWorkflowSessionFocus(second.state, {
      ...START,
      destinationStepId: "step-review",
    });
    expect(newer.requestId).toBe(3);
    expect(newer.state.intent?.destinationStepId).toBe("step-review");
    if (newer.requestId === null) throw new Error("test focus intent did not restart");

    const missingIdentity = bindWorkflowSessionFocus(newer.state, {
      requestId: newer.requestId,
      presentationToken: START.presentationToken,
      entryIdentity: "",
    });
    expect(missingIdentity.intent).toBeNull();
    expect(missingIdentity.request).toBeNull();
  });

  it("keeps a focus request until its desktop or phone consumer acknowledges it", () => {
    const startedState = started();
    const bound = bindWorkflowSessionFocus(startedState.state, {
      requestId: startedState.requestId,
      presentationToken: START.presentationToken,
      entryIdentity: COMMITTED_ROUTE.entry_identity,
    });
    const resolved = reconcileWorkflowSessionFocus(bound, {
      activeTaskId: START.taskId,
      navigationRevision: START.navigationRevision,
      routeMetadata: { workflow_session_route: COMMITTED_ROUTE },
      knownSessionIds: [DESTINATION_SESSION_ID],
    });

    const acknowledged = acknowledgeWorkflowSessionFocus(resolved.state, startedState.requestId);
    expect(acknowledged.request).toBeNull();
  });

  it("uses the live committed route when the response arrives after its task event", () => {
    const startedState = started();
    const bound = bindWorkflowSessionFocus(startedState.state, {
      requestId: startedState.requestId,
      presentationToken: START.presentationToken,
      entryIdentity: COMMITTED_ROUTE.entry_identity,
    });

    const resolved = reconcileWorkflowSessionFocus(bound, {
      activeTaskId: START.taskId,
      navigationRevision: START.navigationRevision,
      routeMetadata: { workflow_session_route: COMMITTED_ROUTE },
      routeUpdatedAt: LIVE_UPDATED_AT,
      responseProjection: {
        metadata: {
          workflow_session_route: { ...COMMITTED_ROUTE, phase: "prepared" },
        },
        updatedAt: RESPONSE_UPDATED_AT,
        entryIdentity: COMMITTED_ROUTE.entry_identity,
      },
      knownSessionIds: [DESTINATION_SESSION_ID],
    });

    expect(resolved.sessionId).toBe(DESTINATION_SESSION_ID);
  });

  it("rejects an obsolete response route after a newer live entry", () => {
    const startedState = started();
    const bound = bindWorkflowSessionFocus(startedState.state, {
      requestId: startedState.requestId,
      presentationToken: START.presentationToken,
      entryIdentity: COMMITTED_ROUTE.entry_identity,
    });
    const newerLiveRoute = {
      ...COMMITTED_ROUTE,
      entry_identity: "entry:00000000000000000043",
      destination_session_id: NEWER_SESSION_ID,
    };

    const result = reconcileWorkflowSessionFocus(bound, {
      activeTaskId: START.taskId,
      navigationRevision: START.navigationRevision,
      routeMetadata: { workflow_session_route: newerLiveRoute },
      routeUpdatedAt: LIVE_UPDATED_AT,
      responseProjection: {
        metadata: { workflow_session_route: COMMITTED_ROUTE },
        updatedAt: RESPONSE_UPDATED_AT,
        entryIdentity: COMMITTED_ROUTE.entry_identity,
      },
      knownSessionIds: [DESTINATION_SESSION_ID, NEWER_SESSION_ID],
    });

    expect(result.sessionId).toBeNull();
    expect(result.state.intent).not.toBeNull();
  });

  it("accepts a committed response only when its entry identity matches the bound move", () => {
    const startedState = started();
    const bound = bindWorkflowSessionFocus(startedState.state, {
      requestId: startedState.requestId,
      presentationToken: START.presentationToken,
      entryIdentity: COMMITTED_ROUTE.entry_identity,
    });

    const result = reconcileWorkflowSessionFocus(bound, {
      activeTaskId: START.taskId,
      navigationRevision: START.navigationRevision,
      routeMetadata: undefined,
      responseProjection: {
        metadata: { workflow_session_route: COMMITTED_ROUTE },
        updatedAt: RESPONSE_UPDATED_AT,
        entryIdentity: "entry:other",
      },
      knownSessionIds: [DESTINATION_SESSION_ID],
    });

    expect(result.sessionId).toBeNull();
    expect(result.state.intent).not.toBeNull();
  });
});
