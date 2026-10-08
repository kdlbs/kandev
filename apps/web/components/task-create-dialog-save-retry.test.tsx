import { useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStore } from "./state-provider";
import { ToastProvider } from "./toast-provider";
import {
  SidebarTaskEditDialog,
  type SidebarTaskEditTarget,
} from "./task/task-session-sidebar-edit";
import { defaultState } from "@/lib/state/default-state";
import type { HydrationState } from "@/lib/state/store";
import { repositoryId, workflowId, workspaceId } from "@/lib/types/ids";
import type { Repository } from "@/lib/types/http";

const TASK_ID = "retry-task";
const WORKSPACE_ID = workspaceId("retry-workspace");
const WORKFLOW_ID = workflowId("retry-workflow");
const REPOSITORY_ID = repositoryId("retry-repository");
const SAVED_TITLE = "Confirmed title";
const SAVED_DESCRIPTION = "Confirmed instructions";
const DRAFT = {
  title: "  Retry title v1  ",
  description: "  Retry instructions v1\nSecond line  ",
};
const WIRE_DRAFT = { title: "Retry title v1", description: "Retry instructions v1\nSecond line" };
const SELECTORS = {
  title: "task-title-input",
  description: "task-description-input",
  dialog: "create-task-dialog",
};
const ACTIONS = { update: "Update", updateTask: "Update task", start: "Start task" };
const TIMESTAMP = "2026-10-08T00:00:00Z";
const TASK_PATH = `/api/v1/tasks/${TASK_ID}`;
const TASK_REPOSITORY = {
  id: "retry-task-repository",
  repository_id: REPOSITORY_ID,
  base_branch: "main",
  position: 0,
};

const repository: Repository = {
  id: REPOSITORY_ID,
  workspace_id: WORKSPACE_ID,
  name: "Retry repository",
  source_type: "local",
  local_path: "",
  provider: "",
  provider_repo_id: "",
  provider_owner: "",
  provider_name: "",
  default_branch: "main",
  worktree_branch_prefix: "",
  pull_before_worktree: false,
  setup_script: "",
  cleanup_script: "",
  dev_script: "",
  copy_files: "",
  created_at: TIMESTAMP,
  updated_at: TIMESTAMP,
};

function makeTarget(started: boolean): SidebarTaskEditTarget {
  return {
    id: TASK_ID,
    title: SAVED_TITLE,
    description: SAVED_DESCRIPTION,
    workflowId: WORKFLOW_ID,
    workflowStepId: "retry-step",
    state: started ? "IN_PROGRESS" : "TODO",
    repositoryId: REPOSITORY_ID,
    repositories: [TASK_REPOSITORY],
  };
}

function initialState(target: SidebarTaskEditTarget): HydrationState {
  return {
    userSettings: { ...defaultState.userSettings, loaded: true },
    settingsData: { ...defaultState.settingsData, agentsLoaded: true, executorsLoaded: true },
    availableAgents: { ...defaultState.availableAgents, loaded: true },
    workflows: {
      ...defaultState.workflows,
      items: [
        {
          id: WORKFLOW_ID,
          workspaceId: WORKSPACE_ID,
          name: "Retry workflow",
        },
      ],
    },
    kanban: {
      ...defaultState.kanban,
      tasks: [{ ...target, position: 0, repositories: [TASK_REPOSITORY] }],
    },
    repositories: {
      ...defaultState.repositories,
      itemsByWorkspaceId: { [WORKSPACE_ID]: [repository] },
      loadedByWorkspaceId: { [WORKSPACE_ID]: true },
    },
    repositorySets: {
      ...defaultState.repositorySets,
      loadedByWorkspaceId: { [WORKSPACE_ID]: true },
    },
  };
}

function EditorConsumer({ initialTarget }: { initialTarget: SidebarTaskEditTarget }) {
  const [target, setTarget] = useState<SidebarTaskEditTarget | null>(initialTarget);
  const confirmed = useAppStore((state) => state.kanban.tasks.find((task) => task.id === TASK_ID));
  return (
    <>
      <output data-testid="confirmed-title">{confirmed?.title}</output>
      <output data-testid="confirmed-description">{confirmed?.description}</output>
      <button onClick={() => setTarget(initialTarget)}>Edit confirmed task</button>
      <SidebarTaskEditDialog
        target={target}
        onTargetChange={setTarget}
        workspaceId={WORKSPACE_ID}
        stepsByWorkflowId={{ [WORKFLOW_ID]: [{ id: "retry-step", title: "Ready" }] }}
      />
    </>
  );
}

function jsonResponse(value: unknown, status = 200): Response {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type PatchOutcome = "failure" | "network" | "stale" | "success";
type PatchPayload = { title?: string; description?: string; repositories?: unknown[] };
const unexpectedRequests: string[] = [];
const windowTimers = new Set<ReturnType<typeof window.setTimeout>>();
const originalAnimations = Object.getOwnPropertyDescriptor(Element.prototype, "getAnimations");

beforeEach(() => {
  // Happy DOM does not render CSS animations; expose its empty animation timeline.
  Object.defineProperty(Element.prototype, "getAnimations", {
    configurable: true,
    value: () => [],
  });
  // Vitest's browser global forwards timers to window; track that owner once.
  const scheduleWindow = window.setTimeout;
  vi.spyOn(window, "setTimeout").mockImplementation((...args) => {
    const timer = scheduleWindow.call(window, ...args);
    windowTimers.add(timer);
    return timer;
  });
});

function requestURL(input: RequestInfo | URL): URL {
  if (input instanceof URL) return input;
  return new URL(typeof input === "string" ? input : input.url, "http://localhost");
}

function patchResponse(outcome: PatchOutcome | undefined, payload: PatchPayload): Response {
  switch (outcome) {
    case "network":
      throw new TypeError("Task edit network failure");
    case "failure":
      return jsonResponse({ error: "Task edit rejected" }, 500);
    case "stale":
      return jsonResponse(
        { error: "Branch policy changed", error_code: "branch_policy_stale" },
        400,
      );
    case "success":
      return jsonResponse({
        id: TASK_ID,
        title: payload.title ?? SAVED_TITLE,
        description: payload.description ?? SAVED_DESCRIPTION,
        state: "TODO",
        workflow_id: WORKFLOW_ID,
      });
    default:
      unexpectedRequests.push(`PATCH ${TASK_PATH}`);
      throw new Error("Unexpected task PATCH");
  }
}

function readResponses(agentEnabled: boolean): Map<string, () => Response> {
  const branches = () =>
    jsonResponse({ branches: [{ name: "main", type: "local" }], total: 1, current_branch: "main" });
  const noConfig = () => new Response(null, { status: 204 });
  return new Map([
    ["/api/v1/user/agent-profile-recent-use", () => jsonResponse([])],
    [
      `/api/v1/workspaces/${WORKSPACE_ID}/repository-sets`,
      () => jsonResponse({ repository_sets: [], total: 0 }),
    ],
    ["/api/v1/jira/config", noConfig],
    ["/api/v1/linear/config", noConfig],
    ["/api/v1/prompts", () => jsonResponse({ prompts: [] })],
    ["/api/v1/agents/Retry%20agent/logo", () => new Response(null, { status: 404 })],
    [
      TASK_PATH,
      () =>
        jsonResponse({
          id: TASK_ID,
          title: SAVED_TITLE,
          description: SAVED_DESCRIPTION,
          depends_on: [],
          blocks: [],
        }),
    ],
    [
      `/api/v1/workspaces/${WORKSPACE_ID}/tasks`,
      () => jsonResponse({ tasks: [], total: 0, page: 1, page_size: 100 }),
    ],
    [
      `/api/v1/workspaces/${WORKSPACE_ID}/repositories/discovery`,
      () =>
        jsonResponse({ roots: [], repositories: [], total: 0, cached: true, refreshing: false }),
    ],
    ["/api/v1/remote-credentials", () => jsonResponse({ auth_specs: [] })],
    [
      "/api/v1/agents",
      () =>
        jsonResponse({
          total: 1,
          agents: [
            {
              id: "retry-agent",
              name: "Retry agent",
              profiles: [
                {
                  id: "retry-profile",
                  name: "Retry profile",
                  agentDisplayName: "Retry agent",
                  enabled: agentEnabled,
                },
              ],
            },
          ],
        }),
    ],
    [`/api/v1/repositories/${REPOSITORY_ID}/branches`, branches],
    [`/api/v1/workspaces/${WORKSPACE_ID}/branches`, branches],
  ]);
}

function mockTransport(outcomes: PatchOutcome[], agentEnabled = false) {
  const patches: PatchPayload[] = [];
  const reads = readResponses(agentEnabled);
  let policyReads = 0;
  let errorReports = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = requestURL(input).pathname;
      const method = init?.method ?? "GET";
      if (path === TASK_PATH && method === "PATCH") {
        const payload = JSON.parse(String(init?.body)) as PatchPayload;
        patches.push(payload);
        return patchResponse(outcomes.shift(), payload);
      }
      if (path === "/api/v1/system/logs/frontend-errors" && method === "POST") {
        errorReports += 1;
        return new Response(null, { status: 204 });
      }
      if (method === "GET") {
        if (path === `/api/v1/repositories/${REPOSITORY_ID}/branch-policies`) {
          policyReads += 1;
          return jsonResponse({ repository_branch_policies: [], total: 0 });
        }
        const read = reads.get(path);
        if (read) return read();
      }
      unexpectedRequests.push(`${method} ${path}`);
      throw new Error(`Unexpected transport request: ${method} ${path}`);
    }),
  );
  return {
    patches,
    get policyReads() {
      return policyReads;
    },
    get errorReports() {
      return errorReports;
    },
  };
}

function submitLabel(started: boolean, withAgent: boolean): string {
  if (withAgent) return ACTIONS.start;
  return started ? ACTIONS.update : ACTIONS.updateTask;
}

async function openEditor(started = false, withAgent = false) {
  const target = makeTarget(started);
  render(
    <StateProvider initialState={initialState(target)}>
      <ToastProvider>
        <TooltipProvider>
          <EditorConsumer initialTarget={target} />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
  await waitFor(() =>
    expect((screen.getByTestId(SELECTORS.title) as HTMLInputElement).value).toBe(SAVED_TITLE),
  );
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: submitLabel(started, withAgent),
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
}

function enterDraft(description = DRAFT.description) {
  fireEvent.change(screen.getByTestId(SELECTORS.title), { target: { value: DRAFT.title } });
  fireEvent.change(screen.getByTestId(SELECTORS.description), {
    target: { value: description },
  });
}

async function submit() {
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: /^Update( task)?$/ }));
  });
}

function assertConfirmed() {
  expect(screen.getByTestId("confirmed-title").textContent).toBe(SAVED_TITLE);
  expect(screen.getByTestId("confirmed-description").textContent).toBe(SAVED_DESCRIPTION);
}

afterEach(async () => {
  cleanup();
  await act(async () => {});
  windowTimers.forEach((timer) => window.clearTimeout(timer));
  windowTimers.clear();
  vi.restoreAllMocks();
  if (originalAnimations)
    Object.defineProperty(Element.prototype, "getAnimations", originalAnimations);
  else Reflect.deleteProperty(Element.prototype, "getAnimations");
  vi.unstubAllGlobals();
  expect(unexpectedRequests.splice(0)).toEqual([]);
});

describe("current task edit save retry", () => {
  // @covers AC-TASKS-EDIT-SAVE-RETRY-001.1 AC-TASKS-EDIT-SAVE-RETRY-001.3
  it("keeps the raw title after an ordinary edit PATCH fails", async () => {
    const transport = mockTransport(["failure"]);
    await openEditor();
    enterDraft();
    await submit();
    expect((screen.queryByTestId(SELECTORS.title) as HTMLInputElement | null)?.value).toBe(
      DRAFT.title,
    );
    expect(screen.getByText("Task edit rejected")).toBeTruthy();
    expect(transport.patches).toEqual([WIRE_DRAFT]);
    assertConfirmed();
    expect(transport.errorReports).toBe(1);
    expect(
      (screen.getByRole("button", { name: ACTIONS.updateTask }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  // @covers AC-TASKS-EDIT-SAVE-RETRY-001.1
  it.each([DRAFT.description, ""])(
    "keeps raw instructions after an ordinary edit PATCH fails: %j",
    async (description) => {
      mockTransport(["failure"]);
      await openEditor();
      enterDraft(description);
      await submit();
      expect(
        (screen.queryByTestId(SELECTORS.description) as HTMLTextAreaElement | null)?.value,
      ).toBe(description);
      assertConfirmed();
    },
  );

  // @covers AC-TASKS-EDIT-SAVE-RETRY-001.1
  it("keeps edits after a transport rejection", async () => {
    mockTransport(["network"]);
    await openEditor();
    enterDraft();
    await submit();
    expect((screen.queryByTestId(SELECTORS.title) as HTMLInputElement | null)?.value).toBe(
      DRAFT.title,
    );
    expect((screen.getByTestId(SELECTORS.description) as HTMLTextAreaElement).value).toBe(
      DRAFT.description,
    );
    expect(screen.getByText("Task edit network failure")).toBeTruthy();
    assertConfirmed();
  });

  // @covers AC-TASKS-EDIT-SAVE-RETRY-001.2
  it("retries the current draft and closes after acknowledgement", async () => {
    const transport = mockTransport(["failure", "success"]);
    await openEditor();
    enterDraft();
    await submit();
    expect(screen.queryByTestId(SELECTORS.dialog)).toBeTruthy();
    assertConfirmed();
    fireEvent.change(screen.getByTestId(SELECTORS.title), {
      target: { value: "  Retry title v2  " },
    });
    fireEvent.change(screen.getByTestId(SELECTORS.description), {
      target: { value: "  Retry instructions v2  " },
    });
    await submit();
    expect(transport.patches[1]).toEqual({
      title: "Retry title v2",
      description: "Retry instructions v2",
    });
    expect(screen.queryByTestId(SELECTORS.dialog)).toBeNull();
  });
});

describe("task edit update-only variants", () => {
  // @covers AC-TASKS-EDIT-SAVE-RETRY-001.4
  it("retains title on started update-only failure", async () => {
    const transport = mockTransport(["failure", "success"]);
    await openEditor(true);
    expect((screen.getByTestId(SELECTORS.description) as HTMLTextAreaElement).disabled).toBe(true);
    fireEvent.change(screen.getByTestId(SELECTORS.title), { target: { value: DRAFT.title } });
    await submit();
    expect((screen.queryByTestId(SELECTORS.title) as HTMLInputElement | null)?.value).toBe(
      DRAFT.title,
    );
    expect(transport.patches).toEqual([{ title: WIRE_DRAFT.title }]);
    await submit();
    expect(transport.patches[1]).toEqual({ title: WIRE_DRAFT.title });
    expect(screen.queryByTestId(SELECTORS.dialog)).toBeNull();
  });

  // @covers AC-TASKS-EDIT-SAVE-RETRY-001.4
  it("retains editable drafts on the update-only alternative", async () => {
    const transport = mockTransport(["failure", "success"], true);
    await openEditor(false, true);
    enterDraft();
    fireEvent.pointerDown(screen.getByTestId("submit-start-agent-chevron"), {
      button: 0,
      ctrlKey: false,
    });
    const firstUpdate = await screen.findByTestId("submit-create-without-agent");
    await act(async () => {
      fireEvent.click(firstUpdate);
    });
    expect((screen.queryByTestId(SELECTORS.title) as HTMLInputElement | null)?.value).toBe(
      DRAFT.title,
    );
    expect((screen.getByTestId(SELECTORS.description) as HTMLTextAreaElement).value).toBe(
      DRAFT.description,
    );
    fireEvent.pointerDown(screen.getByTestId("submit-start-agent-chevron"), {
      button: 0,
      ctrlKey: false,
    });
    const retryUpdate = await screen.findByTestId("submit-create-without-agent");
    await act(async () => {
      fireEvent.click(retryUpdate);
    });
    expect(transport.patches).toEqual([WIRE_DRAFT, WIRE_DRAFT]);
    expect(screen.queryByTestId(SELECTORS.dialog)).toBeNull();
    expect(transport.errorReports).toBe(1);
  });
});

describe("task edit save controls", () => {
  // @covers AC-TASKS-EDIT-SAVE-RETRY-001.5
  it("retains drafts and refreshes a stale branch policy", async () => {
    const transport = mockTransport(["stale"]);
    await openEditor();
    enterDraft();
    const priorReads = transport.policyReads;
    await submit();
    expect((screen.getByTestId(SELECTORS.title) as HTMLInputElement).value).toBe(DRAFT.title);
    expect((screen.getByTestId(SELECTORS.description) as HTMLTextAreaElement).value).toBe(
      DRAFT.description,
    );
    expect(transport.policyReads).toBeGreaterThan(priorReads);
    expect(screen.getByText("Branch policy changed")).toBeTruthy();
    assertConfirmed();
  });

  // @covers AC-TASKS-EDIT-SAVE-RETRY-001.2
  it("closes and acknowledges an ordinary successful edit", async () => {
    const transport = mockTransport(["success"]);
    await openEditor();
    enterDraft();
    await submit();
    expect(transport.patches).toEqual([WIRE_DRAFT]);
    expect(screen.queryByTestId(SELECTORS.dialog)).toBeNull();
    expect(transport.errorReports).toBe(0);
  });

  // @covers AC-TASKS-EDIT-SAVE-RETRY-001.6
  it("explicit cancel retains confirmed values", async () => {
    const transport = mockTransport([]);
    await openEditor();
    enterDraft();
    fireEvent.click(screen.getByTestId("submit-cancel"));
    expect(screen.queryByTestId(SELECTORS.dialog)).toBeNull();
    assertConfirmed();
    fireEvent.click(screen.getByRole("button", { name: "Edit confirmed task" }));
    await waitFor(() =>
      expect((screen.getByTestId(SELECTORS.title) as HTMLInputElement).value).toBe(SAVED_TITLE),
    );
    expect((screen.getByTestId(SELECTORS.description) as HTMLTextAreaElement).value).toBe(
      SAVED_DESCRIPTION,
    );
    expect(transport.patches).toEqual([]);
  });
});
