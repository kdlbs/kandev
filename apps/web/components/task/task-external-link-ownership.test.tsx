import { StrictMode, Suspense, startTransition, useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { SidebarLinkDialogs } from "./task-session-sidebar-dialogs";
import { useSidebarLinkActions } from "./task-session-sidebar-link-actions";
import { TaskExternalLinkDialog } from "./task-external-link-dialog";

const INPUT = "task-external-link-input";
const SAVE = "task-external-link-submit";
const ERROR = "task-external-link-error";
const FAILED = "Original link rejected";
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function SidebarFlow() {
  const store = useAppStoreApi();
  const actions = useSidebarLinkActions(store);
  const [workspace, setWorkspace] = useState("workspace-a");
  return (
    <>
      <button onClick={() => actions.handleLinkJiraTicketTask("task-a", "Task A")}>Link A</button>
      <button onClick={() => actions.handleLinkJiraTicketTask("task-b", "Task B")}>Link B</button>
      <button onClick={() => actions.handleLinkLinearIssueTask("task-a", "Task A")}>Linear</button>
      <button onClick={() => setWorkspace("workspace-b")}>Workspace</button>
      <button onClick={() => actions.setLinkingExternalIssueTask(null)}>Remove</button>
      <button
        onClick={() =>
          store.setState((state) => ({
            kanban: { ...state.kanban, tasks: [{ id: "task-a", title: "Latest title" } as never] },
          }))
        }
      >
        Latest title
      </button>
      <SidebarLinkDialogs actions={actions} repositories={[]} workspaceId={workspace} />
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
function draft(value: string) {
  fireEvent.change(screen.getByTestId(INPUT), { target: { value } });
}
function save() {
  fireEvent.click(screen.getByTestId(SAVE));
}
function input() {
  return screen.getByTestId(INPUT) as HTMLInputElement;
}
async function dismiss() {
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape", code: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
}
function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status });
}

// Only fetch is controlled; provider APIs, coordinator, actual sidebar dialog
// host, Radix, task PATCH and application feedback remain in the request chain.
function transport(holdLookup = false, holdLater = false) {
  let release!: (value: Response) => void;
  const held = new Promise<Response>((resolve) => {
    release = resolve;
  });
  let releaseLater!: (value: Response) => void;
  const laterHeld = new Promise<Response>((resolve) => {
    releaseLater = resolve;
  });
  const patches: Array<{ path: string; body: unknown }> = [];
  const lookups: string[] = [];
  let first = true;
  vi.stubGlobal(
    "fetch",
    vi.fn((value: string, init?: RequestInit) => {
      const url = new URL(value, "http://localhost");
      if (url.pathname.includes("frontend-errors"))
        return Promise.resolve(new Response(null, { status: 204 }));
      if (url.pathname.includes("/jira/tickets/") || url.pathname.includes("/linear/issues/")) {
        lookups.push(url.pathname + url.search);
        if (holdLookup && lookups.length === 1) return held;
        return Promise.resolve(response({ key: url.pathname.split("/").at(-1) }));
      }
      expect(init?.method).toBe("PATCH");
      expect(url.pathname).toMatch(/^\/api\/v1\/tasks\/task-[ab]$/);
      patches.push({ path: url.pathname, body: JSON.parse(init?.body as string) });
      if (holdLater && patches.length === 2) return laterHeld;
      if (!holdLookup && first) {
        first = false;
        return held;
      }
      return Promise.resolve(response({ id: url.pathname.split("/").at(-1) }));
    }),
  );
  return {
    patches,
    lookups,
    releaseLater: async () => {
      await act(async () => releaseLater(response({ id: "task-a" })));
    },
    release: async (failed = false) => {
      await act(async () =>
        release(failed ? response({ error: FAILED }, 400) : response({ key: "APP-1" })),
      );
    },
  };
}
async function begin(request: ReturnType<typeof transport>) {
  press("Link A");
  draft("app-1");
  save();
  await waitFor(() => expect(request.patches).toHaveLength(1));
  expect(request.patches[0]).toEqual({
    path: "/api/v1/tasks/task-a",
    body: { title: "APP-1: Task A" },
  });
  expect(request.lookups[0]).toContain("workspace_id=workspace-a");
}

// @covers AC-TASKS-TASK-ACTIONS-MENU-004.7
it.each(["Link A", "Link B"])(
  "preserves %s reopening after old success and permits explicit Save",
  async (next) => {
    const request = transport();
    renderFlow();
    await begin(request);
    await dismiss();
    press(next);
    draft("APP-2");
    expect(input().disabled).toBe(false);
    await request.release();
    expect(input().value).toBe("APP-2");
    expect(input().disabled).toBe(false);
    expect(screen.queryByTestId(ERROR)).toBeNull();
    expect(screen.getByText("Jira ticket linked")).toBeTruthy();
    expect(request.patches).toHaveLength(1);
    save();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(request.patches[1]).toEqual({
      path: `/api/v1/tasks/${next === "Link A" ? "task-a" : "task-b"}`,
      body: { title: `APP-2: Task ${next === "Link A" ? "A" : "B"}` },
    });
  },
);

it.each(["Link A", "Link B"])(
  "keeps %s draft and old failure feedback separate, with explicit Cancel",
  async (next) => {
    const request = transport();
    renderFlow();
    await begin(request);
    await dismiss();
    press(next);
    draft("APP-2");
    await request.release(true);
    expect(input().value).toBe("APP-2");
    expect(input().disabled).toBe(false);
    expect(screen.queryByTestId(ERROR)).toBeNull();
    expect(screen.getByText(FAILED)).toBeTruthy();
    press("Cancel");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(request.patches).toHaveLength(1);
  },
);

it.each([false, true])("current operation settles normally (failure=%s)", async (failed) => {
  const request = transport();
  renderFlow();
  await begin(request);
  expect(input().disabled).toBe(true);
  await request.release(failed);
  if (failed) {
    expect(screen.getByTestId(ERROR).textContent).toBe(FAILED);
    expect(input().disabled).toBe(false);
    expect(input().value).toBe("app-1");
    draft("APP-2");
    expect(screen.queryByTestId(ERROR)).toBeNull();
    save();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  } else expect(screen.queryByRole("dialog")).toBeNull();
});

it.each([false, true])(
  "unmount preserves admitted completion feedback (failure=%s)",
  async (failed) => {
    const request = transport();
    renderFlow();
    await begin(request);
    press("Remove");
    await request.release(failed);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByText(failed ? FAILED : "Jira ticket linked")).toBeTruthy();
    expect(request.patches).toHaveLength(1);
  },
);

it.each(["Link B", "Linear", "Workspace"])(
  "committed %s replacement owns fresh state during original lookup",
  async (replacement) => {
    const request = transport(true);
    renderFlow();
    press("Link A");
    draft("APP-1");
    save();
    await waitFor(() => expect(request.lookups).toHaveLength(1));
    press(replacement);
    expect(input().value).toBe("");
    expect(input().disabled).toBe(false);
    draft("APP-2");
    press("Latest title");
    await request.release();
    expect(input().value).toBe("APP-2");
    expect(input().disabled).toBe(false);
    expect(request.patches).toEqual([
      { path: "/api/v1/tasks/task-a", body: { title: "APP-1: Latest title" } },
    ]);
    expect(request.lookups[0]).toContain("workspace_id=workspace-a");
  },
);

function RetainedFlow() {
  const [open, setOpen] = useState(true);
  return (
    <>
      <button onClick={() => setOpen(true)}>Reopen</button>
      <TaskExternalLinkDialog
        open={open}
        onOpenChange={setOpen}
        provider="jira"
        task={{ id: "task-a", title: "Task A" }}
        workspaceId="workspace-a"
      />
    </>
  );
}
it("retained closed/open instance does not revive its earlier admission", async () => {
  const request = transport();
  renderFlow(<RetainedFlow />);
  draft("APP-1");
  save();
  await waitFor(() => expect(request.patches).toHaveLength(1));
  await dismiss();
  press("Reopen");
  draft("APP-2");
  expect(input().disabled).toBe(false);
  await request.release();
  expect(input().value).toBe("APP-2");
  expect(input().disabled).toBe(false);
});
it("field-only task updates retain pending opening and latest-title lookup", async () => {
  const request = transport(true);
  renderFlow();
  press("Link A");
  draft("APP-1");
  save();
  await waitFor(() => expect(request.lookups).toHaveLength(1));
  press("Latest title");
  expect(input().disabled).toBe(true);
  await request.release();
  expect(request.patches[0].body).toEqual({ title: "APP-1: Latest title" });
  expect(screen.queryByRole("dialog")).toBeNull();
});

const never = new Promise<void>(() => undefined);
function BlockedRender(): never {
  throw never;
}
function ConcurrentFlow() {
  const [replace, setReplace] = useState(false);
  const [open, setOpen] = useState(true);
  return (
    <>
      <button onClick={() => startTransition(() => setReplace(true))}>Abandon render</button>
      <Suspense fallback={null}>
        <TaskExternalLinkDialog
          open={open}
          onOpenChange={setOpen}
          provider="jira"
          task={{ id: replace ? "task-b" : "task-a", title: "Task A" }}
          workspaceId="workspace-a"
        />
        {replace && <BlockedRender />}
      </Suspense>
    </>
  );
}
it("an uncommitted replacement render cannot revoke the current opening", async () => {
  const request = transport();
  renderFlow(<ConcurrentFlow />);
  draft("APP-1");
  save();
  await waitFor(() => expect(request.patches).toHaveLength(1));
  press("Abandon render");
  expect(input().disabled).toBe(true);
  await request.release();
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.getByText("Jira ticket linked")).toBeTruthy();
});

it.each([false, true])(
  "stale settlement cannot clear a newer pending submit (failure=%s)",
  async (failed) => {
    const request = transport(false, true);
    renderFlow();
    await begin(request);
    await dismiss();
    press("Link A");
    draft("APP-2");
    save();
    await waitFor(() => expect(request.patches).toHaveLength(2));
    await request.release(failed);
    expect(input().value).toBe("APP-2");
    expect(input().disabled).toBe(true);
    expect(screen.queryByTestId(ERROR)).toBeNull();
    await request.releaseLater();
    expect(screen.queryByRole("dialog")).toBeNull();
  },
);

it("current lookup failure remains inline and never starts a task write", async () => {
  const request = transport(true);
  renderFlow();
  press("Link A");
  draft("APP-1");
  save();
  await waitFor(() => expect(request.lookups).toHaveLength(1));
  await request.release(true);
  expect(screen.getByTestId(ERROR).textContent).toBe(FAILED);
  expect(input().disabled).toBe(false);
  expect(request.patches).toHaveLength(0);
});
