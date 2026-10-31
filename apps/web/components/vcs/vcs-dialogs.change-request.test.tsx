import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState, type ReactNode } from "react";
import type { StoreApi } from "zustand";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { AppState } from "@/lib/state/store";
import type { RepositoryProviderRegistration } from "@/lib/plugins/types";
import { pluginRegistry } from "@/lib/plugins/registry";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";
import { openExternalLink } from "@/lib/desktop/external-links";
import { VcsDialogsProvider, useVcsDialogs } from "./vcs-dialogs";

vi.mock("@/lib/api/domains/frontend-error-log-api", () => ({
  scheduleFrontendErrorReport: vi.fn(),
}));
vi.mock("@/lib/desktop/external-links", () => ({
  openExternalLink: vi.fn().mockResolvedValue(undefined),
}));
const TASK = "task-original";
const SESSION = "session-original";
const URL = "https://example.test/pull/27";
const HOST = "fixture-host";
const PLUGIN = "ownership-fixture";
const REPOSITORY = "repo-original";
const success = { success: true, operation: "create_pr", output: "created" };
const failure = { success: false, operation: "create_pr", output: "", error: "provider failed" };
const partial = { ...failure, branch_pushed: true };
const rawTitle = "  Submitted title  ";
const rawBody = "  Submitted body\n ";
const pending: ReturnType<typeof deferred>[] = [];
function deferred() {
  let resolve!: (value: unknown) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { resolve, reject, promise };
}
afterEach(async () => {
  await act(async () => {
    for (const item of pending.splice(0)) item.resolve(success);
  });
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN);
  setWebSocketClient(null);
  vi.clearAllMocks();
});
function Openers() {
  const { openPRDialog } = useVcsDialogs();
  return (
    <>
      <button onClick={() => openPRDialog("original")}>Original</button>
      <button onClick={() => openPRDialog("replacement")}>Replacement</button>
    </>
  );
}
function mount(owningTaskId?: string) {
  const held = deferred();
  pending.push(held);
  const create = vi.fn().mockReturnValueOnce(held.promise).mockResolvedValue(success);
  const request = vi.fn(async (action: string, payload: unknown) => {
    if (action === "worktree.create_pr") return create(payload);
    if (action === "worktree.push") return { success: true, operation: "push", output: "pushed" };
    if (action === "session.git.commits") return { commits: [] };
    if (action === "session.cumulative_diff") return { cumulative_diff: null };
    throw new Error(`Unexpected action ${action}`);
  });
  setWebSocketClient({ request } as unknown as WebSocketClient);
  let store!: StoreApi<AppState>;
  function Initialize({ children }: { children: ReactNode }) {
    store = useAppStoreApi();
    useState(() => {
      store.setState((s) => ({ tasks: { ...s.tasks, activeTaskId: TASK } }));
      return true;
    });
    return children;
  }
  const tree = (sessionId = SESSION, baseBranch = "main", taskId = owningTaskId) => (
    <StateProvider>
      <Initialize>
        <ToastProvider>
          <TooltipProvider>
            <VcsDialogsProvider
              sessionId={sessionId}
              taskId={taskId}
              taskTitle="Original task"
              baseBranch={baseBranch}
            >
              <Openers />
            </VcsDialogsProvider>
          </TooltipProvider>
        </ToastProvider>
      </Initialize>
    </StateProvider>
  );
  const view = render(tree());
  return {
    ...held,
    request,
    create,
    store,
    unmount: view.unmount,
    changeScope: (sid: string, base = "main") => view.rerender(tree(sid, base)),
    changeOwner: (taskId: string) => view.rerender(tree(SESSION, "main", taskId)),
  };
}
function open(name = "Original") {
  fireEvent.click(screen.getByRole("button", { name }));
}
function titleInput() {
  return screen.getByRole("textbox", { name: "Pull Request title" }) as HTMLInputElement;
}
function bodyInput() {
  return screen.getByRole("textbox", { name: "Description" }) as HTMLTextAreaElement;
}
function fill(title = rawTitle, body = rawBody, draft = true) {
  fireEvent.change(titleInput(), { target: { value: title } });
  fireEvent.change(bodyInput(), { target: { value: body } });
  const checkbox = screen.getByRole("checkbox");
  if ((checkbox.getAttribute("aria-checked") === "true") !== draft) fireEvent.click(checkbox);
}
function expectDraft(title = rawTitle, body = rawBody, draft = true) {
  expect(titleInput().value).toBe(title);
  expect(bodyInput().value).toBe(body);
  expect(screen.getByRole("checkbox").getAttribute("aria-checked")).toBe(String(draft));
}
function submit(name = "Create PR") {
  fireEvent.click(screen.getByRole("button", { name }));
}
async function start() {
  const f = mount();
  open();
  fill();
  submit();
  await waitFor(() =>
    expect(f.request).toHaveBeenCalledWith(
      "worktree.create_pr",
      {
        session_id: SESSION,
        title: "Submitted title",
        body: "Submitted body",
        base_branch: "main",
        draft: true,
        repo: "original",
      },
      120000,
    ),
  );
  expect(screen.queryByRole("dialog")).toBeNull();
  return f;
}

// @covers AC-INTEGRATIONS-NATIVE-CREATE-001.1, .2, .3
// Only the nondeterministic request transport is replaced in built-in creation.
describe("native create completion ownership", () => {
  it.each(["success", "failure", "rejection", "partial"])(
    "preserves replacement after old %s",
    async (outcome) => {
      const f = await start();
      open("Replacement");
      fill("  New raw title  ", "  New raw body\n", false);
      titleInput().focus();
      titleInput().setSelectionRange(3, 7);
      await act(async () => {
        if (outcome === "rejection") f.reject(new Error("request rejected"));
        else if (outcome === "partial") f.resolve(partial);
        else f.resolve(outcome === "success" ? success : failure);
      });
      expectDraft("  New raw title  ", "  New raw body\n", false);
      expect(document.activeElement).toBe(titleInput());
      expect([titleInput().selectionStart, titleInput().selectionEnd]).toEqual([3, 7]);
      expect(screen.getByRole("dialog").textContent).toContain("replacement");
      expect(screen.queryByRole("button", { name: "Retry PR" })).toBeNull();
      submit();
      await waitFor(() => expect(f.create).toHaveBeenCalledTimes(2));
      expect(f.create).toHaveBeenLastCalledWith({
        session_id: SESSION,
        repo: "replacement",
        title: "New raw title",
        body: "New raw body",
        base_branch: "main",
        draft: false,
      });
    },
  );
  it.each(["same", "away-back"])(
    "preserves equal-value %s replacement and Cancel",
    async (scope) => {
      const f = await start();
      if (scope === "away-back") {
        open("Replacement");
        fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      }
      open();
      fill();
      await act(async () => f.resolve(partial));
      expectDraft();
      expect(screen.queryByRole("button", { name: "Retry PR" })).toBeNull();
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(f.create).toHaveBeenCalledTimes(1);
    },
  );
  it.each(["session", "task", "base"])(
    "retires authority across %s away/back without reopening",
    async (kind) => {
      const f = await start();
      if (kind === "task") {
        act(() => f.store.setState((s) => ({ tasks: { ...s.tasks, activeTaskId: "other" } })));
        act(() => f.store.setState((s) => ({ tasks: { ...s.tasks, activeTaskId: TASK } })));
      } else {
        f.changeScope(kind === "session" ? "other" : SESSION, kind === "base" ? "develop" : "main");
        f.changeScope(SESSION);
      }
      await act(async () => f.resolve(partial));
      expect(screen.queryByRole("dialog")).toBeNull();
    },
  );
  it("keeps original association and draft feedback after active task changes", async () => {
    const f = await start();
    act(() =>
      f.store.setState((s) => ({ tasks: { ...s.tasks, activeTaskId: "replacement-task" } })),
    );
    open("Replacement");
    fill("New", "Body", false);
    await act(async () => f.resolve({ ...success, pr_url: URL }));
    expectDraft("New", "Body", false);
    expect(f.store.getState().pendingPrUrlByTaskId.byTaskId).toEqual({
      [TASK]: { original: URL },
    });
    expect(openExternalLink).toHaveBeenCalledWith(URL);
    expect(screen.getByText("Draft PR created")).toBeTruthy();
  });
  it("revokes completion on unmount", async () => {
    const f = await start();
    f.unmount();
    await act(async () => f.resolve({ ...success, pr_url: URL }));
    expect(openExternalLink).not.toHaveBeenCalled();
    expect(f.store.getState().pendingPrUrlByTaskId.byTaskId).toEqual({});
  });
});
// @covers AC-INTEGRATIONS-NATIVE-CREATE-001.4, .5
describe("owned creation controls", () => {
  it.each(["success", "failure", "rejection"])(
    "keeps ordinary %s close/reset policy",
    async (outcome) => {
      const f = await start();
      await act(async () => {
        if (outcome === "rejection") f.reject(new Error("request rejected"));
        else f.resolve(outcome === "success" ? success : failure);
      });
      expect(screen.queryByRole("dialog")).toBeNull();
      open();
      expectDraft("Original task", "");
      expect(
        screen.getByText(outcome === "success" ? "Draft PR created" : "Create PR failed"),
      ).toBeTruthy();
    },
  );
  it("retains owned partial failure's raw draft and Retry control", async () => {
    const f = await start();
    await act(async () => f.resolve(partial));
    expectDraft();
    submit("Retry PR");
    await waitFor(() => expect(f.create).toHaveBeenCalledTimes(2));
  });
});

function registerProvider(
  f: ReturnType<typeof mount>,
  createChangeRequest: NonNullable<RepositoryProviderRegistration["createChangeRequest"]>,
) {
  pluginRegistry.setDeclaredRepositoryProviderIds(PLUGIN, [HOST]);
  pluginRegistry.registerRepositoryProvider(PLUGIN, {
    id: HOST,
    label: "Fixture",
    listRepositories: async () => [],
    matchesURL: () => false,
    listBranches: async () => [],
    inspectURL: async () => null,
    createChangeRequest,
  });
  act(() =>
    f.store.setState((state) => ({
      taskSessions: {
        ...state.taskSessions,
        items: {
          [SESSION]: { task_id: TASK } as never,
          "session-other": { task_id: TASK } as never,
        },
      },
      kanban: {
        ...state.kanban,
        tasks: [
          {
            id: TASK,
            workspaceId: "workspace",
            repositories: [
              { repository_id: REPOSITORY, position: 0 },
              { repository_id: "repo-replacement", position: 1 },
            ],
          } as never,
        ],
      },
      repositories: {
        ...state.repositories,
        itemsByWorkspaceId: {
          workspace: [
            {
              id: REPOSITORY,
              workspace_id: "workspace",
              name: "original",
              provider: HOST,
            } as never,
            {
              id: "repo-replacement",
              workspace_id: "workspace",
              name: "replacement",
              provider: HOST,
            } as never,
          ],
        },
      },
    })),
  );
}
function pushes(f: ReturnType<typeof mount>) {
  return f.request.mock.calls.filter(([action]) => action === "worktree.push");
}

// @covers AC-INTEGRATIONS-NATIVE-CREATE-001.1, .4
// The registered callback is the external boundary; push and target resolution are real.
describe("registered provider creation ownership", () => {
  it("retries owned partial create without pushing again", async () => {
    const f = mount();
    const callback = vi
      .fn<NonNullable<RepositoryProviderRegistration["createChangeRequest"]>>()
      .mockRejectedValueOnce(new Error("host unavailable"))
      .mockResolvedValue({ url: URL });
    registerProvider(f, callback);
    open();
    fill();
    submit();
    await screen.findByRole("button", { name: "Retry PR" });
    expectDraft();
    expect(pushes(f)).toHaveLength(1);
    submit("Retry PR");
    await waitFor(() => expect(callback).toHaveBeenCalledTimes(2));
    expect(pushes(f)).toHaveLength(1);
    expect(callback).toHaveBeenLastCalledWith(
      expect.objectContaining({
        taskId: TASK,
        sessionId: SESSION,
        workspaceId: "workspace",
        repositoryId: REPOSITORY,
        title: "Submitted title",
        body: "Submitted body",
        baseBranch: "main",
        draft: true,
      }),
    );
  });
  it("keeps captured target while old partial completion cannot skip replacement repository push", async () => {
    const f = mount();
    const held = deferred();
    pending.push(held);
    const callback = vi
      .fn<NonNullable<RepositoryProviderRegistration["createChangeRequest"]>>()
      .mockImplementationOnce(async () => {
        await held.promise;
        return { url: URL };
      })
      .mockResolvedValue({ url: "https://example.test/pull/28" });
    registerProvider(f, callback);
    open();
    fill();
    submit();
    await waitFor(() => expect(callback).toHaveBeenCalledTimes(1));
    open("Replacement");
    fill("  Next  ", "Next body", false);
    await act(async () => held.reject(new Error("host unavailable")));
    expectDraft("  Next  ", "Next body", false);
    expect(screen.queryByRole("button", { name: "Retry PR" })).toBeNull();
    submit();
    await waitFor(() => expect(callback).toHaveBeenCalledTimes(2));
    expect(pushes(f).map(([, payload]) => payload)).toEqual([
      { session_id: SESSION, repo: "original", force: false, set_upstream: true },
      { session_id: SESSION, repo: "replacement", force: false, set_upstream: true },
    ]);
    expect(callback.mock.calls.map(([input]) => input.repositoryId)).toEqual([
      REPOSITORY,
      "repo-replacement",
    ]);
  });
});

// @covers AC-INTEGRATIONS-NATIVE-CREATE-001.1, .3, .4
it("associates the session task even when another task is active at admission", async () => {
  const f = mount();
  act(() =>
    f.store.setState((s) => ({
      taskSessions: {
        ...s.taskSessions,
        items: { [SESSION]: { task_id: "session-task" } as never },
      },
      tasks: { ...s.tasks, activeTaskId: "unrelated-active-task" },
    })),
  );
  open();
  fill();
  submit();
  await waitFor(() => expect(f.create).toHaveBeenCalledTimes(1));
  await act(async () => f.resolve({ ...success, pr_url: URL }));
  expect(f.store.getState().pendingPrUrlByTaskId.byTaskId).toEqual({
    "session-task": { original: URL },
  });
});

it("does not reuse retained provider push state in another session", async () => {
  const f = mount();
  const callback = vi
    .fn<NonNullable<RepositoryProviderRegistration["createChangeRequest"]>>()
    .mockRejectedValueOnce(new Error("host unavailable"))
    .mockResolvedValue({ url: URL });
  registerProvider(f, callback);
  open();
  fill();
  submit();
  await screen.findByRole("button", { name: "Retry PR" });
  expect(pushes(f)).toHaveLength(1);
  f.changeScope("session-other");
  expectDraft();
  expect(screen.queryByRole("button", { name: "Retry PR" })).toBeNull();
  submit();
  await waitFor(() => expect(callback).toHaveBeenCalledTimes(2));
  expect(pushes(f)).toHaveLength(2);
  expect(pushes(f)[1][1]).toMatchObject({ session_id: "session-other", repo: "original" });
});

// @covers AC-INTEGRATIONS-NATIVE-CREATE-001.1, .3
it.each([false, true])(
  "keeps known task ownership while session metadata ready=%s",
  async (ready) => {
    const f = mount(TASK);
    const items: AppState["taskSessions"]["items"] = {};
    if (ready) items[SESSION] = { task_id: "hydrated-task" } as never;
    act(() =>
      f.store.setState((state) => ({
        tasks: { ...state.tasks, activeTaskId: "unrelated-active-task" },
        taskSessions: {
          ...state.taskSessions,
          items,
        },
      })),
    );
    open();
    fill();
    submit();
    await waitFor(() => expect(f.create).toHaveBeenCalledTimes(1));
    await act(async () => f.resolve({ ...success, pr_url: URL }));
    expect(f.store.getState().pendingPrUrlByTaskId.byTaskId).toEqual({
      [ready ? "hydrated-task" : TASK]: { original: URL },
    });
  },
);

it("retires a known task owner across away/back before session hydration", async () => {
  const f = mount(TASK);
  act(() =>
    f.store.setState((state) => ({
      tasks: { ...state.tasks, activeTaskId: "unrelated-active-task" },
    })),
  );
  open();
  fill();
  submit();
  await waitFor(() => expect(f.create).toHaveBeenCalledTimes(1));
  f.changeOwner("other-known-task");
  f.changeOwner(TASK);
  await act(async () => f.resolve(partial));
  expect(screen.queryByRole("dialog")).toBeNull();
});
