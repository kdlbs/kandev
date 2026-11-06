import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { useParams, useSearchParams } from "@/lib/routing/client-router";
import {
  clearNavigationBlockerForTests,
  setNavigationBlocker,
  type NavigationIntent,
} from "@/lib/routing/navigation-guard";
import type { AppState } from "@/lib/state/store";
import { useSubtaskSubmit } from "./use-subtask-submit";

const PARENT_TASK_ID = "parent-task";
const PARENT_SESSION_ID = "parent-session";
const CHILD_TASK_ID = "child-task";
const CHILD_SESSION_ID = "child-session";

const { mockCreateTask } = vi.hoisted(() => ({ mockCreateTask: vi.fn() }));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  createTask: mockCreateTask,
}));

type SubmitOptions = Parameters<typeof useSubtaskSubmit>[0];

function makeSubmitOptions(overrides: Partial<SubmitOptions> = {}): SubmitOptions {
  return {
    fs: {
      useRemote: false,
      remoteRepos: [],
      prInfoByUrl: {},
      repositories: [],
      discoveredRepositories: [],
      agentProfileId: "",
      executorProfileId: "",
    } as unknown as SubmitOptions["fs"],
    parentTaskId: PARENT_TASK_ID,
    defaultProfileId: "default-profile",
    workspaceId: "workspace-1",
    workflowId: "workflow-1",
    availableRepositories: [],
    attachments: [],
    resolvePrompt: () => "do the work",
    title: "Manual title",
    setIsCreating: vi.fn(),
    onClose: vi.fn(),
    workspaceMode: "inherit_parent",
    ...overrides,
  };
}

function makeInitialState(): Partial<AppState> {
  return {
    tasks: {
      activeTaskId: PARENT_TASK_ID,
      activeSessionId: PARENT_SESSION_ID,
    },
    taskSessions: {
      items: {
        [PARENT_SESSION_ID]: {
          id: PARENT_SESSION_ID,
          task_id: PARENT_TASK_ID,
          state: "WAITING_FOR_INPUT",
        },
      },
    },
  } as unknown as Partial<AppState>;
}

function navigationWrapper(initialState: Partial<AppState>) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider initialState={initialState}>
        <ToastProvider>{children}</ToastProvider>
      </StateProvider>
    );
  };
}

function renderSubmitHarness(options: SubmitOptions, initialState = makeInitialState()) {
  return renderHook(
    () => ({
      params: useParams(),
      searchParams: useSearchParams(),
      submit: useSubtaskSubmit(options),
      store: useAppStoreApi(),
    }),
    { wrapper: navigationWrapper(initialState) },
  );
}

async function submit(result: {
  current: { submit: { handleSubmit: (e: never) => Promise<void> } };
}) {
  await act(async () => {
    await result.current.submit.handleSubmit({ preventDefault: vi.fn() } as never);
  });
}

function setLocation(path: string) {
  window.history.replaceState({}, "", path);
}

beforeEach(() => {
  vi.clearAllMocks();
  clearNavigationBlockerForTests();
  setLocation(`/t/${PARENT_TASK_ID}`);
  mockCreateTask.mockResolvedValue({ id: CHILD_TASK_ID, session_id: CHILD_SESSION_ID });
});

afterEach(() => {
  clearNavigationBlockerForTests();
});

describe("useSubtaskSubmit SPA navigation", () => {
  it("opens the created child in route subscribers", async () => {
    const options = makeSubmitOptions();
    const { result } = renderSubmitHarness(options);

    await submit(result);

    expect(window.location.pathname).toBe(`/t/${CHILD_TASK_ID}`);
    expect(window.location.search).toBe(`?sessionId=${CHILD_SESSION_ID}`);
    await waitFor(() => expect(result.current.params.taskId).toBe(CHILD_TASK_ID));
    expect(result.current.searchParams.get("sessionId")).toBe(CHILD_SESSION_ID);
    expect(result.current.store.getState().tasks.activeTaskId).toBe(CHILD_TASK_ID);
    expect(result.current.store.getState().tasks.activeSessionId).toBe(CHILD_SESSION_ID);
  });

  it("closes the dialog and commits the selection after the router notification", async () => {
    const events: string[] = [];
    const options = makeSubmitOptions({
      onClose: () => events.push(`close:${window.location.pathname}`),
    });
    const intents: NavigationIntent[] = [];
    setNavigationBlocker((intent) => intents.push(intent));
    const { result } = renderSubmitHarness(options);

    await submit(result);

    expect(events).toEqual([`close:/t/${PARENT_TASK_ID}`]);
    expect(window.location.pathname).toBe(`/t/${PARENT_TASK_ID}`);
    expect(result.current.store.getState().tasks.activeTaskId).toBe(PARENT_TASK_ID);
    expect(result.current.store.getState().tasks.activeSessionId).toBe(PARENT_SESSION_ID);
    expect(mockCreateTask).toHaveBeenCalledOnce();

    act(() => intents.shift()?.proceed());

    expect(window.location.pathname).toBe(`/t/${CHILD_TASK_ID}`);
    expect(result.current.store.getState().tasks.activeTaskId).toBe(CHILD_TASK_ID);
    expect(result.current.store.getState().tasks.activeSessionId).toBe(CHILD_SESSION_ID);
    await waitFor(() => expect(result.current.params.taskId).toBe(CHILD_TASK_ID));
  });

  it("keeps the parent selection and never recreates the child when navigation is cancelled", async () => {
    const intents: NavigationIntent[] = [];
    setNavigationBlocker((intent) => intents.push(intent));
    const { result } = renderSubmitHarness(makeSubmitOptions());

    await submit(result);
    act(() => intents.shift()?.cancel());

    expect(window.location.pathname).toBe(`/t/${PARENT_TASK_ID}`);
    expect(result.current.params.taskId).toBe(PARENT_TASK_ID);
    expect(result.current.store.getState().tasks.activeTaskId).toBe(PARENT_TASK_ID);
    expect(result.current.store.getState().tasks.activeSessionId).toBe(PARENT_SESSION_ID);
    expect(mockCreateTask).toHaveBeenCalledOnce();
  });

  it("keeps a replacement navigation for the created child", async () => {
    const historyLength = window.history.length;
    const { result } = renderSubmitHarness(makeSubmitOptions());

    await submit(result);

    expect(window.history.length).toBe(historyLength);
    expect(result.current.store.getState().tasks.pinnedSessionId).toBe(CHILD_SESSION_ID);
  });

  it("uses the returned primary session when session_id is absent", async () => {
    mockCreateTask.mockResolvedValue({ id: CHILD_TASK_ID, primary_session_id: "primary-session" });
    const { result } = renderSubmitHarness(makeSubmitOptions());

    await submit(result);

    expect(window.location.pathname).toBe(`/t/${CHILD_TASK_ID}`);
    expect(window.location.search).toBe("?sessionId=primary-session");
    expect(result.current.store.getState().tasks.activeSessionId).toBe("primary-session");
  });

  it("prefers session_id over primary_session_id", async () => {
    mockCreateTask.mockResolvedValue({
      id: CHILD_TASK_ID,
      session_id: CHILD_SESSION_ID,
      primary_session_id: "primary-session",
    });
    const { result } = renderSubmitHarness(makeSubmitOptions());

    await submit(result);

    expect(window.location.search).toBe(`?sessionId=${CHILD_SESSION_ID}`);
    expect(result.current.store.getState().tasks.activeSessionId).toBe(CHILD_SESSION_ID);
  });

  it("closes without navigation when creation returns no session", async () => {
    mockCreateTask.mockResolvedValue({ id: CHILD_TASK_ID });
    const options = makeSubmitOptions();
    const { result } = renderSubmitHarness(options);

    await submit(result);

    expect(options.onClose).toHaveBeenCalledOnce();
    expect(window.location.pathname).toBe(`/t/${PARENT_TASK_ID}`);
    expect(result.current.store.getState().tasks.activeTaskId).toBe(PARENT_TASK_ID);
    expect(result.current.store.getState().tasks.activeSessionId).toBe(PARENT_SESSION_ID);
  });

  it("keeps the current selection when creation fails", async () => {
    mockCreateTask.mockRejectedValueOnce(new Error("request failed"));
    const options = makeSubmitOptions();
    const { result } = renderSubmitHarness(options);

    await submit(result);

    expect(options.onClose).not.toHaveBeenCalled();
    expect(window.location.pathname).toBe(`/t/${PARENT_TASK_ID}`);
    expect(result.current.store.getState().tasks.activeTaskId).toBe(PARENT_TASK_ID);
    expect(result.current.store.getState().tasks.activeSessionId).toBe(PARENT_SESSION_ID);
  });
});
