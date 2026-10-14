import { createRef, useLayoutEffect } from "react";
import type { StoreApi } from "zustand";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, vi } from "vitest";
import type { Window as HappyDOMWindow } from "happy-dom";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import {
  ChatInputArea,
  useChatPanelHandlers,
  useSubmitHandler,
} from "@/components/task/chat/chat-input-area";
import type { ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";
import { useChatPanelState } from "@/components/task/chat/use-chat-panel-state";
import { PlanPanelHeader } from "@/components/task/task-plan-panel-header";
import { defaultState } from "@/lib/state/default-state";
import type { AppState, HydrationState } from "@/lib/state/store";
import type { AgentProfile } from "@/lib/types/agent-profile";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type TaskSession,
  type TaskPlan,
} from "@/lib/types/http";
import {
  getChatDraftText,
  getChatDraftContent,
  getChatDraftAttachments,
  setChatDraftText,
  setChatDraftContent,
} from "@/lib/local-storage";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";

export const TASK = "implementation-task";
export const FIRST = "planning-session";
export const SECOND = "other-planning-session";
export const FRESH = "fresh-implementation-session";
// i18n-exempt: synthetic submitted message used by the transport regression fixture.
export const ORIGINAL = "Implement with the original instruction";
// i18n-exempt: synthetic unsent message used by the draft regression fixture.
export const NEXT = "Keep this newer unsent instruction";
const PROFILE = "implementation-profile" as AgentProfile["id"];
const timestamp = "2026-10-08T00:00:00Z";
const plan: TaskPlan = {
  id: "implementation-plan",
  task_id: TASK,
  // i18n-exempt: synthetic task-plan data in a test fixture.
  title: "Plan",
  content: "Implement the bounded change",
  created_by: "agent",
  created_at: timestamp,
  updated_at: timestamp,
};
const profile: AgentProfile = {
  id: PROFILE,
  name: "Profile",
  agentId: "agent",
  agentDisplayName: "Agent",
  model: "model",
  allowIndexing: false,
  autoApprove: false,
  cliFlags: [],
  cliPassthrough: false,
  createdAt: timestamp,
  updatedAt: timestamp,
};
const sessions = [FIRST, SECOND, FRESH].map(
  (id) =>
    ({
      id: toSessionId(id),
      task_id: toTaskId(TASK),
      agent_profile_id: PROFILE,
      executor_id: "executor",
      queue_incarnation_id: `incarnation-${id}`,
      state: "WAITING_FOR_INPUT",
      metadata: { plan_mode: id !== FRESH },
      started_at: timestamp,
      updated_at: timestamp,
    }) as TaskSession,
);

export function deferred() {
  let resolve!: (value: unknown) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<unknown>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function initialState(): HydrationState {
  const loaded = Object.fromEntries(sessions.map((s) => [s.id, true]));
  return {
    connection: { ...defaultState.connection, status: "connected" },
    tasks: { ...defaultState.tasks, activeTaskId: TASK, activeSessionId: FIRST },
    kanban: {
      ...defaultState.kanban,
      workflowId: "workflow",
      // i18n-exempt: synthetic workflow-step data in a test fixture.
      steps: [{ id: "planning", title: "Planning", position: 0, color: "blue" }],
      tasks: [
        {
          id: TASK,
          workspaceId: "workspace",
          workflowId: "workflow",
          workflowStepId: "planning",
          // i18n-exempt: synthetic task data in a test fixture.
          title: "Task",
          position: 0,
        },
      ],
    },
    settingsAgents: {
      items: [
        {
          id: "agent",
          name: "Agent",
          supports_mcp: true,
          profiles: [profile],
          created_at: timestamp,
          updated_at: timestamp,
        },
      ],
    },
    agentProfiles: {
      ...defaultState.agentProfiles,
      items: [
        {
          id: PROFILE,
          // i18n-exempt: synthetic agent-profile data in a test fixture.
          label: "Profile",
          agent_id: "agent",
          agent_name: "Agent",
          cli_passthrough: false,
          model: "model",
        },
      ],
    },
    settingsData: { ...defaultState.settingsData, agentsLoaded: true, executorsLoaded: true },
    userSettings: { ...defaultState.userSettings, loaded: true },
    prompts: { items: [], loaded: true, loading: false },
    editors: { ...defaultState.editors, loaded: true, folderOpeningAvailable: false },
    taskSessions: {
      ...defaultState.taskSessions,
      items: Object.fromEntries(sessions.map((s) => [s.id, s])),
    },
    taskSessionsByTask: {
      ...defaultState.taskSessionsByTask,
      itemsByTaskId: { [TASK]: sessions },
      loadedByTaskId: { [TASK]: true },
    },
    chatInput: {
      ...defaultState.chatInput,
      planModeBySessionId: { [FIRST]: true, [SECOND]: true, [FRESH]: false },
    },
    turns: {
      ...defaultState.turns,
      bySession: Object.fromEntries(sessions.map((s) => [s.id, []])),
      loadedBySession: loaded,
    },
    taskPlans: {
      ...defaultState.taskPlans,
      byTaskId: { [TASK]: plan },
      loadedByTaskId: { [TASK]: true },
      commentsByTaskId: { [TASK]: { task_id: TASK, plan_id: plan.id, revision: 0, comments: [] } },
      commentsLoadedByTaskId: { [TASK]: true },
    },
    previewFeedback: { ...defaultState.previewFeedback, loadedByTaskId: { [TASK]: true } },
  };
}

type Options = { header?: boolean; fixedSession?: boolean; editorKey?: number };
export type Runtime = {
  ref: ReturnType<typeof createRef<ChatInputContainerHandle>>;
  store?: StoreApi<AppState>;
};
function Surface({ runtime, options }: { runtime: Runtime; options: Options }) {
  const store = useAppStoreApi();
  useLayoutEffect(() => {
    runtime.store = store;
  }, [runtime, store]);
  const active = useAppStore((s) => s.tasks.activeSessionId);
  const currentPlan = useAppStore((s) => s.taskPlans.byTaskId[TASK]);
  const panel = useChatPanelState({
    sessionId: options.fixedSession ? FIRST : active,
    taskId: TASK,
    disableWorkbenchEffects: true,
  });
  const submit = useSubmitHandler(panel);
  const handlers = useChatPanelHandlers(panel.resolvedSessionId, runtime.ref, {
    enableFocusShortcut: false,
  });
  return (
    <>
      {options.header && (
        <PlanPanelHeader
          taskId={TASK}
          plan={currentPlan ?? null}
          draftContent={plan.content}
          hasUnsavedChanges={false}
          activeSessionId={FIRST}
          revisions={[]}
          isLoadingRevisions={false}
          isSaving={false}
          isAgentBusy={false}
          attemptSave={async () => currentPlan ?? null}
          onOpenRevisions={() => {}}
          onRevert={async () => null}
          loadRevisionContent={async () => ""}
          previewRevisionId={null}
          setPreviewRevision={() => {}}
          comparePair={[null, null]}
          toggleCompareSelection={() => {}}
          clearComparePair={() => {}}
        />
      )}
      <ChatInputArea
        chatInputRef={runtime.ref}
        clarificationKey={options.editorKey ?? 0}
        onClarificationResolved={() => {}}
        handleSubmit={submit.handleSubmit}
        handleCancelTurn={handlers.handleCancelTurn}
        showRequestChangesTooltip={false}
        panelState={panel}
        isSending={submit.isSending}
        hideAgentControls
        hideSessionsDropdown
      />
    </>
  );
}

export function mount(options: Options = {}) {
  const runtime: Runtime = { ref: createRef<ChatInputContainerHandle>() };
  const tree = (next = options) => (
    <StateProvider initialState={initialState()}>
      <ToastProvider>
        <TooltipProvider>
          <Surface runtime={runtime} options={next} />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>
  );
  const view = render(tree());
  return { runtime, view, tree };
}

export function save(id = FIRST, text = ORIGINAL) {
  setChatDraftText(id, text);
  setChatDraftContent(id, {
    type: "doc",
    content: [{ type: "paragraph", content: [{ type: "text", text }] }],
  });
}
export function saveRich() {
  save();
  setChatDraftText(FIRST, `\`${ORIGINAL}\``);
  setChatDraftContent(FIRST, {
    type: "doc",
    content: [
      { type: "paragraph", content: [{ type: "text", text: ORIGINAL, marks: [{ type: "code" }] }] },
    ],
  });
}
export function saved(id = FIRST) {
  return {
    text: getChatDraftText(id),
    content: getChatDraftContent(id),
    attachments: getChatDraftAttachments(id),
  };
}
export async function flush() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(100);
  });
}
export async function settle(gate: ReturnType<typeof deferred>, value: unknown = {}) {
  await act(async () => {
    gate.resolve(value);
    await gate.promise;
  });
  await flush();
}
export function edit(runtime: Runtime, html = NEXT) {
  const visible = runtime.ref.current!.getTextareaElement()!.textContent!.length;
  act(() => runtime.ref.current!.insertText(html, 1, visible + 1));
}
export function expectEditor(runtime: Runtime, markdown: string, visible = markdown) {
  expect.soft(runtime.ref.current!.getValue()).toBe(markdown);
  expect.soft(runtime.ref.current!.getTextareaElement()!.textContent).toBe(visible);
}
export async function clickImplement(fresh = false, header = false) {
  if (!fresh)
    fireEvent.click(
      screen.getByTestId(header ? "plan-toolbar-implement-button" : "implement-plan-button"),
    );
  else {
    fireEvent.pointerDown(
      screen.getByTestId(
        header ? "plan-toolbar-implement-menu-trigger" : "implement-plan-menu-trigger",
      ),
      { button: 0, ctrlKey: false, pointerType: "mouse" },
    );
    await flush();
    fireEvent.click(
      screen.getByTestId(
        header ? "plan-toolbar-implement-fresh-menu-item" : "implement-fresh-menu-item",
      ),
    );
  }
  await flush();
}

function setupFetch(unexpected: string[], serverPlanMode: Map<string, boolean>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/mcp-config")) return Response.json({ enabled: false, servers: {} });
      if (url.endsWith("/turns")) return Response.json({ turns: [] });
      const session = sessions.find((s) => url.endsWith(`/task-sessions/${s.id}`));
      if (session)
        return Response.json({
          session: {
            ...session,
            metadata: { ...session.metadata, plan_mode: serverPlanMode.get(session.id) },
          },
        });
      if (url.endsWith(`/tasks/${TASK}`))
        return Response.json({
          task: {
            id: TASK,
            workspace_id: "workspace",
            workflow_id: "workflow",
            workflow_step_id: "planning",
            // i18n-exempt: synthetic task data in a test fixture.
            title: "Task",
            position: 0,
            state: "TODO",
          },
        });
      if (url.endsWith("/ci-options"))
        return Response.json({
          task_id: TASK,
          auto_fix_enabled: false,
          auto_merge_enabled: false,
          auto_fix_prompt_override: null,
          effective_auto_fix_prompt: "",
          using_default_prompt: true,
          updated_at: timestamp,
          pr_states: [],
          pr_options: [],
        });
      if (url.includes("/usage/turns?")) return Response.json({ turns: [] });
      if (url.endsWith("/usage"))
        return Response.json({
          scope: "session",
          scope_id: FIRST,
          tokens_in: 0,
          tokens_cached_read: 0,
          tokens_cached_write: 0,
          tokens_out: 0,
          tokens_thought: 0,
          tokens_total: 0,
          cost_subcents: 0,
          event_count: 0,
          estimated_event_count: 0,
          unpriced_event_count: 0,
          output_tokens_complete: true,
          first_event_at: null,
          last_event_at: null,
        });
      if (url.endsWith("/environment/live")) return Response.json({ environment: null });
      if (url.includes("frontend")) return new Response(null, { status: 204 });
      unexpected.push(`HTTP ${url}`);
      throw new Error(`Unexpected fixture fetch: ${url}`);
    }),
  );
}

export function setupTransport() {
  const gates = new Map<string, ReturnType<typeof deferred>>();
  const unexpected: string[] = [];
  const ready = Promise.resolve();
  const serverPlanMode = new Map<string, boolean>(sessions.map((s) => [s.id, s.id !== FRESH]));
  const request = vi.fn(async (action: string, params: Record<string, unknown>) => {
    const held = gates.get(action);
    if (held) return held.promise;
    if (action === "session.set_plan_mode") {
      serverPlanMode.set(String(params.session_id), Boolean(params.enabled));
      return {};
    }
    if (action === "message.add" || action === "session.set_primary") return {};
    if (action === "session.launch")
      return { success: true, task_id: TASK, session_id: FRESH, state: "WAITING_FOR_INPUT" };
    if (action === "task.plan.implementation_started")
      return {
        ...plan,
        implementation_started_at: timestamp,
        implementation_started_session_id: params.session_id,
        implementation_started_by: "user",
      };
    if (action === "message.list") return { messages: [], has_more: false };
    if (action === "message.queue.get")
      return {
        ...params,
        entries: [],
        count: 0,
        max: 20,
        merge_enabled: true,
        auto_run: true,
        status_epoch: "fixture",
        status_generation: 1,
      };
    if (action === "task.plan.get") return plan;
    if (action === "task.plan.comments.list")
      return { task_id: TASK, plan_id: plan.id, revision: 0, comments: [] };
    if (action === "task.preview_feedback.list") return { task_id: TASK, revision: 0, items: [] };
    if (action === "github.task_pr.sync") return { prs: [], permanent: true };
    unexpected.push(`WS ${action}`);
    throw new Error(`Unexpected fixture transport: ${action}`);
  });
  const noop = () => {};
  const subscribe = () => noop;
  setWebSocketClient({
    request,
    getStatus: () => "connected",
    subscribe,
    subscribeSession: subscribe,
    subscribeSessionWithReady: () => ({ ready, unsubscribe: noop }),
    getSessionSubscriptionReadiness: () => ready,
    on: subscribe,
    off: noop,
    onConnectionStatus: subscribe,
    onRawSessionEvent: subscribe,
    registerCoreSessionRecovery: subscribe,
    subscribeUser: subscribe,
  } as unknown as WebSocketClient);
  setupFetch(unexpected, serverPlanMode);
  return {
    request,
    unexpected,
    hold(action: string) {
      const gate = deferred();
      gates.set(action, gate);
      return gate;
    },
    release(action: string) {
      gates.delete(action);
    },
    gates,
  };
}

export function lifecycle() {
  let transport: ReturnType<typeof setupTransport>;
  const viewport = (window as unknown as HappyDOMWindow).happyDOM;
  const initialWidth = window.innerWidth;
  const animations = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "getAnimations");
  beforeEach(() => {
    sessionStorage.clear();
    localStorage.clear();
    vi.useFakeTimers();
    if (!animations)
      Object.defineProperty(HTMLElement.prototype, "getAnimations", {
        configurable: true,
        value: () => [],
      });
    transport = setupTransport();
  });
  afterEach(async () => {
    for (const gate of transport.gates.values()) gate.resolve({});
    cleanup();
    await flush();
    setWebSocketClient(null);
    expect(transport.unexpected).toEqual([]);
    vi.clearAllTimers();
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    if (!animations)
      delete (HTMLElement.prototype as unknown as Record<string, unknown>).getAnimations;
    viewport.setWindowSize({ width: initialWidth, height: 900 });
    sessionStorage.clear();
    localStorage.clear();
  });
  return {
    transport: () => transport,
    width(value: number) {
      viewport.setWindowSize({ width: value, height: 900 });
      fireEvent(window, new Event("resize"));
    },
  };
}
