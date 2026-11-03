import { StrictMode, Suspense, startTransition, useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { AppState } from "@/lib/state/store";
import type { Repository } from "@/lib/types/http";
import { TaskMRLinkDialog } from "@/components/gitlab/task-mr-link-dialog";
import { SidebarLinkDialogs } from "./task-session-sidebar-dialogs";
import { useSidebarLinkActions } from "./task-session-sidebar-link-actions";

const ORIGINAL = "https://gitlab.example.test/team/repo/-/merge_requests/8";
const LATER = "  https://gitlab.example.test/team/other/-/merge_requests/9  ";
const WORKSPACE_A = "workspace-a";
const ORIGINAL_REPOSITORY = "Original repository";
const OTHER_REPOSITORY = "Other repository";
const FAILED = "Original MR link rejected";
const repositories = [
  { id: "repo-a", name: ORIGINAL_REPOSITORY },
  { id: "repo-b", name: OTHER_REPOSITORY },
] as Repository[];
const links = repositories.map((repo) => ({ repository_id: repo.id }));
let store: StoreApi<AppState>;
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function SidebarFlow() {
  store = useAppStoreApi();
  const actions = useSidebarLinkActions(store, [
    { id: "task-a", title: "Task A", repositories: links },
    { id: "task-b", title: "Task B", repositories: links },
  ] as never);
  const [workspace, setWorkspace] = useState(WORKSPACE_A);
  return (
    <>
      <button onClick={() => actions.handleLinkMergeRequestTask("task-a")}>Link A</button>
      <button onClick={() => actions.handleLinkMergeRequestTask("task-b")}>Link B</button>
      <button onClick={() => setWorkspace("workspace-b")}>Workspace B</button>
      <button onClick={() => setWorkspace(WORKSPACE_A)}>Workspace A</button>
      <button onClick={() => actions.setLinkingMergeRequestTask(null)}>Remove</button>
      <SidebarLinkDialogs actions={actions} repositories={repositories} workspaceId={workspace} />
    </>
  );
}
function renderFlow(child = <SidebarFlow />) {
  return render(
    <StrictMode>
      <StateProvider>
        <ToastProvider>{child}</ToastProvider>
      </StateProvider>
    </StrictMode>,
  );
}
function press(name: string) {
  fireEvent.click(screen.getByText(name, { exact: true, selector: "button" }));
}
function input() {
  return screen.getByLabelText("Merge request URL") as HTMLInputElement;
}
function draft(value: string) {
  fireEvent.change(input(), { target: { value } });
}
function save() {
  press("Link merge request");
}
function isBusy() {
  return screen.getByRole("button", { name: "Linking…" });
}
async function chooseRepository(name: string) {
  fireEvent.click(screen.getByRole("combobox", { name: "Task repository" }));
  fireEvent.click(await screen.findByRole("option", { name }));
}
async function dismiss() {
  press("Cancel");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
}
function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status });
}
// Only transport is held. Coordinator, Radix, providers, API and store are real.
function transport() {
  const requests: Array<{
    workspace: string;
    body: { task_id: string; mr_url: string; repository_id?: string };
    release: (value: Response) => void;
  }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((value: string, init?: RequestInit) => {
      const url = new URL(value, "http://localhost");
      if (url.pathname.includes("frontend-errors"))
        return Promise.resolve(new Response(null, { status: 204 }));
      expect(url.pathname).toBe("/api/v1/gitlab/task-mrs");
      expect(init?.method).toBe("POST");
      return new Promise<Response>((release) =>
        requests.push({
          workspace: url.searchParams.get("workspace_id")!,
          body: JSON.parse(init!.body as string),
          release,
        }),
      );
    }),
  );
  return {
    requests,
    async settle(index = 0, failed = false) {
      const request = requests[index];
      const association = {
        id: `association-${index}`,
        task_id: request.body.task_id,
        repository_id: request.body.repository_id,
        mr_url: request.body.mr_url,
        host: "https://gitlab.example.test",
        project_path: "team/repo",
        mr_iid: index + 8,
        state: "open",
      };
      await act(async () =>
        request.release(failed ? response({ error: FAILED }, 400) : response(association)),
      );
      return association;
    },
  };
}
async function begin(request: ReturnType<typeof transport>, open = true) {
  if (open) press("Link A");
  draft(` ${ORIGINAL} `);
  save();
  await waitFor(() => expect(request.requests).toHaveLength(1));
  expect(request.requests[0]).toMatchObject({
    workspace: WORKSPACE_A,
    body: { task_id: "task-a", repository_id: "repo-a", mr_url: ORIGINAL },
  });
}
function originalAssociation() {
  return store.getState().taskMRs.byWorkspaceId[WORKSPACE_A]?.["task-a"]?.[0];
}
function assertDraft(value = LATER) {
  expect(input().value).toBe(value);
  expect(screen.getByRole("button", { name: "Link merge request" }).hasAttribute("disabled")).toBe(
    false,
  );
}
// @covers AC-TASKS-TASK-ACTIONS-MENU-004.8
it.each(["Link A", "Link B"])(
  "old success preserves %s reopening and explicit Save",
  async (next) => {
    const request = transport();
    renderFlow();
    await begin(request);
    await dismiss();
    press(next);
    draft(LATER);
    input().focus();
    input().setSelectionRange(4, 11);
    const node = input();
    await request.settle();
    expect(originalAssociation()).toMatchObject({ id: "association-0", repository_id: "repo-a" });
    assertDraft();
    expect(input()).toBe(node);
    expect(document.activeElement).toBe(node);
    expect([node.selectionStart, node.selectionEnd]).toEqual([4, 11]);
    save();
    await waitFor(() => expect(request.requests).toHaveLength(2));
    expect(request.requests[1]).toMatchObject({
      workspace: WORKSPACE_A,
      body: { task_id: next === "Link A" ? "task-a" : "task-b", mr_url: LATER.trim() },
    });
    await request.settle(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  },
);
it.each(["Link A", "Link B"])(
  "old failure preserves %s reopening with truthful feedback and Cancel",
  async (next) => {
    const request = transport();
    renderFlow();
    await begin(request);
    await dismiss();
    press(next);
    draft(LATER);
    await request.settle(0, true);
    assertDraft();
    expect(screen.getByText(FAILED)).toBeTruthy();
    expect(originalAssociation()).toBeUndefined();
    await dismiss();
    expect(request.requests).toHaveLength(1);
  },
);
it.each([false, true])(
  "current completion succeeds or fails and retries (failure=%s)",
  async (failed) => {
    const request = transport();
    renderFlow();
    await begin(request);
    expect(isBusy().hasAttribute("disabled")).toBe(true);
    await request.settle(0, failed);
    if (failed) {
      assertDraft(` ${ORIGINAL} `);
      expect(screen.getByText(FAILED)).toBeTruthy();
      save();
      await waitFor(() => expect(request.requests).toHaveLength(2));
      await request.settle(1);
    }
    expect(originalAssociation()).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  },
);
it.each(["Link B", "Workspace B"])(
  "committed %s replacement and return never revive earlier authority",
  async (replacement) => {
    const request = transport();
    renderFlow();
    await begin(request);
    press(replacement);
    expect(input().value).toBe("");
    press(replacement === "Link B" ? "Link A" : "Workspace A");
    draft(` ${ORIGINAL} `);
    await request.settle();
    assertDraft(` ${ORIGINAL} `);
    expect(originalAssociation()).toBeTruthy();
  },
);
it.each([false, true])(
  "repository replacement retires old completion (return=%s)",
  async (returnToOriginal) => {
    const request = transport();
    renderFlow();
    await begin(request);
    await chooseRepository(OTHER_REPOSITORY);
    if (returnToOriginal) await chooseRepository(ORIGINAL_REPOSITORY);
    await request.settle();
    assertDraft(` ${ORIGINAL} `);
    expect(screen.getByRole("combobox").textContent).toBe(
      returnToOriginal ? ORIGINAL_REPOSITORY : OTHER_REPOSITORY,
    );
    expect(originalAssociation()?.repository_id).toBe("repo-a");
    save();
    await waitFor(() => expect(request.requests).toHaveLength(2));
    expect(request.requests[1].body.repository_id).toBe(returnToOriginal ? "repo-a" : "repo-b");
    await request.settle(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  },
);
it.each([false, true])(
  "unmount retains original publication or failure feedback (failure=%s)",
  async (failed) => {
    const request = transport();
    renderFlow();
    await begin(request);
    press("Remove");
    await request.settle(0, failed);
    expect(screen.queryByRole("dialog")).toBeNull();
    if (failed) expect(screen.getByText(FAILED)).toBeTruthy();
    else expect(originalAssociation()).toBeTruthy();
  },
);
function RetainedFlow() {
  store = useAppStoreApi();
  const [open, setOpen] = useState(true);
  return (
    <>
      <button onClick={() => setOpen(true)}>Reopen</button>
      <TaskMRLinkDialog
        open={open}
        onOpenChange={setOpen}
        taskId="task-a"
        workspaceId={WORKSPACE_A}
        repositories={repositories}
        taskRepositories={links}
      />
    </>
  );
}
it("retained closed/open form accepts an equal raw draft independently", async () => {
  const request = transport();
  renderFlow(<RetainedFlow />);
  await begin(request, false);
  await dismiss();
  press("Reopen");
  expect(input().value).toBe("");
  draft(` ${ORIGINAL} `);
  await request.settle();
  assertDraft(` ${ORIGINAL} `);
  await dismiss();
});
it.each([false, true])(
  "old finally cannot clear a newer pending request (failure=%s)",
  async (failed) => {
    const request = transport();
    renderFlow(<RetainedFlow />);
    await begin(request, false);
    await dismiss();
    press("Reopen");
    draft(LATER);
    save();
    await waitFor(() => expect(request.requests).toHaveLength(2));
    await request.settle(0, failed);
    expect(input().value).toBe(LATER);
    expect(isBusy().hasAttribute("disabled")).toBe(true);
    fireEvent.keyDown(input(), { key: "Enter", code: "Enter" });
    expect(request.requests).toHaveLength(2);
    await request.settle(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  },
);
const never = new Promise<void>(() => undefined);
function BlockedRender(): never {
  throw never;
}
function ConcurrentFlow() {
  store = useAppStoreApi();
  const [replace, setReplace] = useState(false);
  const [open, setOpen] = useState(true);
  return (
    <>
      <button onClick={() => startTransition(() => setReplace(true))}>Abandon render</button>
      <Suspense fallback={null}>
        <TaskMRLinkDialog
          open={open}
          onOpenChange={setOpen}
          taskId={replace ? "task-b" : "task-a"}
          workspaceId={WORKSPACE_A}
          repositories={repositories}
          taskRepositories={links}
        />
        {replace && <BlockedRender />}
      </Suspense>
    </>
  );
}
it("uncommitted replacement cannot revoke the committed opening", async () => {
  const request = transport();
  renderFlow(<ConcurrentFlow />);
  await begin(request, false);
  press("Abandon render");
  expect(isBusy().hasAttribute("disabled")).toBe(true);
  await request.settle();
  expect(originalAssociation()).toBeTruthy();
  expect(screen.queryByRole("dialog")).toBeNull();
});

it.each([false, true])(
  "old repository request cannot clear newer repository busy state (failure=%s)",
  async (failed) => {
    const request = transport();
    renderFlow();
    await begin(request);
    await chooseRepository(OTHER_REPOSITORY);
    draft(LATER);
    save();
    await waitFor(() => expect(request.requests).toHaveLength(2));
    await request.settle(0, failed);
    expect(input().value).toBe(LATER);
    expect(isBusy().hasAttribute("disabled")).toBe(true);
    expect(screen.getByRole("combobox").textContent).toBe(OTHER_REPOSITORY);
    await request.settle(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  },
);

it("duplicate activation before render admits only one current request", async () => {
  const request = transport();
  renderFlow();
  press("Link A");
  draft(ORIGINAL);
  const button = screen.getByRole("button", { name: "Link merge request" });
  act(() => {
    button.click();
    button.click();
  });
  expect(request.requests).toHaveLength(1);
  await request.settle();
  expect(screen.queryByRole("dialog")).toBeNull();
});

function HydrationFlow() {
  store = useAppStoreApi();
  const [hydrated, setHydrated] = useState(false);
  const [open, setOpen] = useState(true);
  return (
    <>
      <button onClick={() => setHydrated(true)}>Hydrate repository</button>
      <TaskMRLinkDialog
        open={open}
        onOpenChange={setOpen}
        taskId="task-a"
        workspaceId={WORKSPACE_A}
        repositories={hydrated ? repositories : []}
        taskRepositories={hydrated ? links : []}
      />
    </>
  );
}

it.each([false, true])(
  "passive repository hydration retains pending ownership (failure=%s)",
  async (failed) => {
    const request = transport();
    renderFlow(<HydrationFlow />);
    draft(` ${ORIGINAL} `);
    save();
    await waitFor(() => expect(request.requests).toHaveLength(1));
    expect(request.requests[0]).toMatchObject({
      workspace: WORKSPACE_A,
      body: { task_id: "task-a", mr_url: ORIGINAL },
    });
    expect(request.requests[0].body).not.toHaveProperty("repository_id");
    press("Hydrate repository");
    expect(input().value).toBe(` ${ORIGINAL} `);
    expect(screen.getByRole("combobox").textContent).toBe(ORIGINAL_REPOSITORY);
    expect(isBusy().hasAttribute("disabled")).toBe(true);
    fireEvent.keyDown(input(), { key: "Enter", code: "Enter" });
    expect(request.requests).toHaveLength(1);
    await request.settle(0, failed);
    if (failed) {
      assertDraft(` ${ORIGINAL} `);
      expect(screen.getByText(FAILED)).toBeTruthy();
      save();
      await waitFor(() => expect(request.requests).toHaveLength(2));
      expect(request.requests[1].body.repository_id).toBe("repo-a");
      await request.settle(1);
    }
    expect(originalAssociation()).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  },
);
