/* eslint-disable sonarjs/no-duplicate-string -- Row ids and copy strings repeat so each case reads on its own. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { ApiError } from "@/lib/api/client";
import type { ActivityItem } from "@/lib/api/domains/coordinator-activity-api";

const mocks = vi.hoisted(() => ({
  listActivity: vi.fn(),
  undoActivity: vi.fn(),
  listMembers: vi.fn(),
  replace: vi.fn(),
  search: { current: "" },
  wsHandlers: [] as Array<(message: { payload: Record<string, string> }) => void>,
}));

vi.mock("@/lib/api/domains/coordinator-activity-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/coordinator-activity-api")>()),
  listActivity: mocks.listActivity,
  undoActivity: mocks.undoActivity,
}));
vi.mock("@/lib/api/domains/team-access-api", () => ({ listWorkspaceMembers: mocks.listMembers }));
vi.mock("@/lib/ws/connection", () => ({
  useWebSocketClient: () => ({
    on: (_event: string, handler: (typeof mocks.wsHandlers)[number]) => {
      mocks.wsHandlers.push(handler);
      return () => undefined;
    },
  }),
}));
vi.mock("@/components/state-provider", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/components/state-provider")>()),
  useAppStore: (selector: (s: unknown) => unknown) =>
    selector({ connection: { status: "connected" } }),
}));
vi.mock("@/lib/routing/client-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/routing/client-router")>()),
  usePathname: () => "/coordinator/queue",
  useSearchParams: () => new URLSearchParams(mocks.search.current),
  useRouter: () => ({ replace: mocks.replace, push: vi.fn() }),
}));

import { WhatItDid, type WhatItDidProps } from "./what-it-did";

function row(id: string, over: Partial<ActivityItem> = {}): ActivityItem {
  return {
    id,
    coordinator_id: "co-1",
    workspace_id: "ws-1",
    action_class: "move",
    outcome: "approved",
    authorization: "requires_approval",
    target_task_id: "task-1",
    proposal_id: null,
    actor_user_id: "u-1",
    reason_code: null,
    detail: "",
    edited: false,
    refusal_count: 1,
    undone_at: null,
    undone_by: null,
    undo_of_id: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    undoable: true,
    target_task_identifier: "KAN-1",
    from_step_id: null,
    ...over,
  };
}

const page = (rows: ActivityItem[], next: string | null = null) => ({ rows, next_cursor: next });

function props(over: Partial<WhatItDidProps> = {}): WhatItDidProps {
  return {
    workspaceId: "ws-1",
    coordinatorId: "co-1",
    canManage: true,
    tasks: [],
    tasksLoadedAt: 1,
    tasksError: false,
    stepNameByWorkflowStep: new Map(),
    ...over,
  };
}

async function mount(
  rows: ActivityItem[],
  over: Partial<WhatItDidProps> = {},
  next: string | null = null,
) {
  mocks.listActivity.mockResolvedValue(page(rows, next));
  const view = render(<WhatItDid {...props(over)} />);
  await screen.findByTestId(rows.length ? `activity-row-${rows[0].id}` : "activity-empty");
  return view;
}

const rowEl = (id: string) => screen.getByTestId(`activity-row-${id}`);

beforeEach(() => {
  mocks.listActivity.mockReset();
  mocks.undoActivity.mockReset();
  mocks.listMembers.mockReset();
  mocks.replace.mockReset();
  mocks.search.current = "";
  mocks.wsHandlers.length = 0;
  mocks.listMembers.mockResolvedValue({
    members: [
      { user_id: "u-1", display_name: "Ana" },
      { user_id: "u-2", display_name: "Bo" },
      { user_id: "u-3", display_name: "" },
    ],
  });
});
afterEach(cleanup);

describe("authorisation copy", () => {
  it("shows approved by a named member, edited, and no-person forms", async () => {
    await mount([
      row("a1"),
      row("a2", { edited: true }),
      row("a3", { actor_user_id: null }),
      row("a4", { actor_user_id: "u-3" }),
      row("a5", { actor_user_id: "u-9" }),
    ]);
    await waitFor(() => expect(within(rowEl("a1")).getByText("Approved by Ana")).toBeTruthy());
    expect(within(rowEl("a2")).getByText("Approved by Ana, with edits")).toBeTruthy();
    expect(within(rowEl("a3")).getByText("Approved")).toBeTruthy();
    expect(within(rowEl("a4")).getByText("Approved")).toBeTruthy();
    expect(within(rowEl("a5")).getByText("Approved by a former member")).toBeTruthy();
  });

  it("uses the no-person form while members load and when the read failed", async () => {
    mocks.listMembers.mockReturnValue(new Promise(() => undefined));
    await mount([row("a1")]);
    expect(within(rowEl("a1")).getByText("Approved")).toBeTruthy();
    cleanup();
    mocks.listMembers.mockRejectedValue(new Error("x"));
    await mount([row("a1", { actor_user_id: "u-9" })]);
    await waitFor(() => expect(mocks.listMembers).toHaveBeenCalled());
    expect(within(rowEl("a1")).getByText("Approved")).toBeTruthy();
  });

  it("prefixes rejected and failed details and falls back without one", async () => {
    await mount([
      row("r1", { outcome: "rejected", detail: "too risky" }),
      row("r2", { outcome: "rejected" }),
      row("f1", { outcome: "failed", detail: "boom" }),
      row("f2", { outcome: "failed", detail: "   " }),
    ]);
    expect(within(rowEl("r1")).getByText("Rejected: too risky")).toBeTruthy();
    expect(within(rowEl("r2")).getAllByText("Rejected").length).toBeGreaterThan(0);
    expect(within(rowEl("f1")).getByText("Failed: boom")).toBeTruthy();
    expect(within(rowEl("f2")).getAllByText("Failed").length).toBeGreaterThan(0);
  });

  it("renders each refusal reason, an unknown code, no code and the repeat count", async () => {
    const refused = {
      outcome: "refused",
      authorization: "denied",
      actor_user_id: null,
      undoable: false,
    };
    await mount([
      row("x1", { ...refused, reason_code: "binding_invalid" }),
      row("x2", { ...refused, reason_code: "not_in_profile" }),
      row("x3", { ...refused, reason_code: "policy_denied", refusal_count: 3 }),
      row("x4", { ...refused, reason_code: "new_thing" }),
      row("x5", { ...refused, reason_code: null }),
    ]);
    expect(within(rowEl("x1")).getByText("Its tool settings could not be read.")).toBeTruthy();
    expect(
      within(rowEl("x2")).getByText("It called something it is not allowed to use."),
    ).toBeTruthy();
    expect(within(rowEl("x3")).getByText("A manager has set this action to Denied.")).toBeTruthy();
    expect(within(rowEl("x3")).getByText("Denied x 3")).toBeTruthy();
    expect(within(rowEl("x1")).getByText("Denied")).toBeTruthy();
    expect(within(rowEl("x4")).getByText("Refused. new_thing")).toBeTruthy();
    expect(within(rowEl("x5")).getByText("Refused.")).toBeTruthy();
  });
});

describe("task references", () => {
  it("links the identifier, or Open task when the snapshots hold the task", async () => {
    await mount(
      [row("a1"), row("a2", { target_task_identifier: null, target_task_id: "t-held" })],
      {
        tasks: [{ id: "t-held" } as never],
      },
    );
    expect(within(rowEl("a1")).getByText("KAN-1").closest("a")).toBeTruthy();
    expect(within(rowEl("a2")).getByText("Open task").closest("a")).toBeTruthy();
  });

  it("shows Task no longer available only when trusted and absent", async () => {
    const noId = { target_task_identifier: null };
    await mount([row("a1", noId), row("a2", { ...noId, target_task_id: null })]);
    expect(within(rowEl("a1")).getByText("Task no longer available")).toBeTruthy();
    expect(within(rowEl("a2")).queryByText("Task no longer available")).toBeNull();
    cleanup();
    await mount([row("a1", noId)], { tasksLoadedAt: undefined });
    expect(screen.queryByText("Task no longer available")).toBeNull();
    cleanup();
    await mount([row("a1", noId)], { tasksError: true });
    expect(screen.queryByText("Task no longer available")).toBeNull();
  });
});

describe("undo cell", () => {
  it("shows No undo on message and resume rows, also for readers, and nothing on rejected create", async () => {
    await mount(
      [
        row("m1", { action_class: "message", undoable: false }),
        row("m2", {
          action_class: "resume",
          outcome: "refused",
          authorization: "denied",
          undoable: false,
        }),
        row("c1", { action_class: "create_task", outcome: "rejected", undoable: false }),
      ],
      { canManage: false },
    );
    expect(within(rowEl("m1")).getByText("No undo")).toBeTruthy();
    expect(within(rowEl("m2")).getByText("No undo")).toBeTruthy();
    expect(within(rowEl("c1")).queryByText("No undo")).toBeNull();
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
  });

  it("shows the undone state with no Undo, and an empty cell for the undone outcome row", async () => {
    await mount([
      row("c1", {
        action_class: "create_task",
        undone_at: new Date().toISOString(),
        undone_by: "u-2",
      }),
      row("u1", { outcome: "undone", undoable: false, undo_of_id: "c1" }),
    ]);
    await waitFor(() => expect(within(rowEl("c1")).getByText(/^Undone by Bo, /)).toBeTruthy());
    expect(within(rowEl("c1")).queryByTestId("activity-undo-c1")).toBeNull();
    expect(
      within(within(rowEl("u1")).getByTestId("activity-undo-cell")).queryByRole("button"),
    ).toBeNull();
  });

  it("hides Undo from readers", async () => {
    await mount([row("a1")], { canManage: false });
    expect(screen.queryByTestId("activity-undo-a1")).toBeNull();
  });
});

describe("undo dialog", () => {
  const open = (id: string) => fireEvent.click(screen.getByTestId(`activity-undo-${id}`));

  it("uses the create sentence with the identifier", async () => {
    await mount([row("c1", { action_class: "create_task" })]);
    open("c1");
    expect((await screen.findByTestId("activity-undo-dialog-text")).textContent).toBe(
      "The task KAN-1 will be archived. Any agent working on it will be stopped.",
    );
  });

  it("uses the move sentences with and without the step name and identifier", async () => {
    const steps = new Map([["wf:step-1", "Backlog"]]);
    await mount([row("a1", { from_step_id: "step-1" })], { stepNameByWorkflowStep: steps });
    open("a1");
    expect((await screen.findByTestId("activity-undo-dialog-text")).textContent).toBe(
      "The task KAN-1 will move back to Backlog.",
    );
    cleanup();
    await mount([row("a1", { from_step_id: "gone" })], { stepNameByWorkflowStep: steps });
    open("a1");
    expect((await screen.findByTestId("activity-undo-dialog-text")).textContent).toBe(
      "The task KAN-1 will move back to the step it came from.",
    );
    cleanup();
    await mount([row("a1", { from_step_id: "step-1", target_task_identifier: null })], {
      stepNameByWorkflowStep: steps,
    });
    open("a1");
    expect((await screen.findByTestId("activity-undo-dialog-text")).textContent).toBe(
      "This task will move back to Backlog.",
    );
    cleanup();
    await mount([row("a1", { target_task_identifier: null })]);
    open("a1");
    expect((await screen.findByTestId("activity-undo-dialog-text")).textContent).toBe(
      "This task will move back to the step it came from.",
    );
    cleanup();
    await mount([row("c1", { action_class: "create_task", target_task_identifier: null })]);
    open("c1");
    expect((await screen.findByTestId("activity-undo-dialog-text")).textContent).toBe(
      "This task will be archived. Any agent working on it will be stopped.",
    );
  });

  it("focuses Cancel, sends nothing on Cancel, Escape or Enter, and one request on confirm", async () => {
    await mount([row("a1")]);
    open("a1");
    const cancel = await screen.findByRole("button", { name: "Cancel" });
    await waitFor(() => expect(document.activeElement).toBe(cancel));
    fireEvent.keyDown(cancel, { key: "Enter" });
    fireEvent.keyUp(cancel, { key: "Enter" });
    expect(mocks.undoActivity).not.toHaveBeenCalled();
    fireEvent.keyDown(document.activeElement as Element, { key: "Escape" });
    await waitFor(() => expect(screen.queryByTestId("activity-undo-dialog-text")).toBeNull());
    expect(mocks.undoActivity).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByTestId("activity-undo-a1")),
    );

    open("a1");
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(mocks.undoActivity).not.toHaveBeenCalled();

    mocks.undoActivity.mockResolvedValue({});
    open("a1");
    fireEvent.click(await screen.findByTestId("activity-undo-confirm"));
    await waitFor(() => expect(mocks.undoActivity).toHaveBeenCalledTimes(1));
    expect(mocks.undoActivity).toHaveBeenCalledWith("ws-1", "co-1", "a1");
  });

  it("re-reads after a 200 so the row flips to Undone, including rows beyond page 1", async () => {
    mocks.listActivity.mockResolvedValueOnce(page([row("a1")], "cur-1"));
    render(<WhatItDid {...props()} />);
    await screen.findByTestId("activity-row-a1");
    mocks.listActivity.mockResolvedValueOnce(page([row("b1")], null));
    fireEvent.click(screen.getByTestId("activity-load-more"));
    await screen.findByTestId("activity-row-b1");
    mocks.undoActivity.mockResolvedValue({});
    mocks.listActivity
      .mockResolvedValueOnce(page([row("a1")], "cur-1"))
      .mockResolvedValueOnce(
        page([row("b1", { undone_at: new Date().toISOString(), undone_by: "u-2" })]),
      );
    open("b1");
    fireEvent.click(await screen.findByTestId("activity-undo-confirm"));
    await waitFor(() => expect(within(rowEl("b1")).getByText(/^Undone by Bo, /)).toBeTruthy());
  });
});

describe("undo refusals", () => {
  async function refuse(error: unknown, over: Partial<ActivityItem> = {}) {
    await mount([row("a1", over)]);
    mocks.undoActivity.mockRejectedValue(error);
    fireEvent.click(screen.getByTestId("activity-undo-a1"));
    fireEvent.click(await screen.findByTestId("activity-undo-confirm"));
  }
  const conflict = (body: Record<string, string>) => new ApiError("x", 409, body);

  it.each([
    ["step_full", "The step it came from is full."],
    ["step_deleted", "The step it came from no longer exists."],
    ["step_done", "The step it came from is now a finishing step."],
    ["agent_running", "An agent is working on it. Stop it, then undo."],
    ["other", "It has moved since"],
  ])("shows the message for undo_conflict %s", async (reason, text) => {
    await refuse(conflict({ code: "undo_conflict", reason }));
    expect(await within(rowEl("a1")).findByText(text)).toBeTruthy();
    expect(screen.getByTestId("activity-undo-a1")).toBeTruthy();
  });

  it("shows the moved message when the reason is absent", async () => {
    await refuse(conflict({ code: "undo_conflict" }));
    expect(await within(rowEl("a1")).findByText("It has moved since")).toBeTruthy();
  });

  it("is silent on already_undone and re-reads", async () => {
    await mount([row("a1")]);
    mocks.undoActivity.mockRejectedValue(conflict({ code: "already_undone" }));
    mocks.listActivity.mockResolvedValue(
      page([row("a1", { undone_at: new Date().toISOString(), undone_by: "u-2" })]),
    );
    fireEvent.click(screen.getByTestId("activity-undo-a1"));
    fireEvent.click(await screen.findByTestId("activity-undo-confirm"));
    await waitFor(() => expect(within(rowEl("a1")).getByText(/^Undone by Bo, /)).toBeTruthy());
    expect(screen.queryByTestId("activity-undo-message")).toBeNull();
  });

  it("shows a retry message on a server error and keeps the button", async () => {
    await refuse(new ApiError("x", 500, null));
    expect(await within(rowEl("a1")).findByText("Undo failed. Try again.")).toBeTruthy();
    expect(screen.getByTestId("activity-undo-a1").getAttribute("aria-disabled")).toBe("false");
  });

  it("shows the not undoable message and re-reads", async () => {
    await mount([row("a1")]);
    mocks.undoActivity.mockRejectedValue(conflict({ code: "not_undoable" }));
    mocks.listActivity.mockResolvedValue(
      page([row("a1", { undoable: false, action_class: "message" })]),
    );
    fireEvent.click(screen.getByTestId("activity-undo-a1"));
    fireEvent.click(await screen.findByTestId("activity-undo-confirm"));
    expect(await within(rowEl("a1")).findByText("This can no longer be undone")).toBeTruthy();
    expect(within(rowEl("a1")).getByText("No undo")).toBeTruthy();
  });

  it("shows a section notice on 404 when the row is still listed", async () => {
    await refuse(new ApiError("x", 404, null));
    expect((await screen.findByTestId("activity-notice")).textContent).toBe(
      "This action is no longer listed.",
    );
  });
});

// eslint-disable-next-line max-lines-per-function -- one fixture serves every list lifecycle case
describe("list lifecycle", () => {
  it("shows a load failure with Retry rather than the empty text", async () => {
    mocks.listActivity.mockRejectedValueOnce(new Error("x"));
    render(<WhatItDid {...props()} />);
    await screen.findByTestId("activity-load-failed");
    expect(screen.queryByTestId("activity-empty")).toBeNull();
    mocks.listActivity.mockResolvedValueOnce(page([row("a1")]));
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await screen.findByTestId("activity-row-a1");
  });

  it("shows the empty and filtered-empty texts", async () => {
    await mount([]);
    expect(screen.getByTestId("activity-empty").textContent).toBe("It has not done anything yet.");
    cleanup();
    mocks.search.current = "class=move";
    await mount([]);
    expect(screen.getByTestId("activity-empty").textContent).toBe("Nothing matches this filter.");
    expect(mocks.listActivity.mock.calls.at(-1)?.[2]).toEqual({ class: "move" });
  });

  it("selects All for an unrecognised class value", async () => {
    mocks.search.current = "class=bogus";
    await mount([row("a1")]);
    expect(mocks.listActivity.mock.calls[0][2]).toEqual({ class: undefined });
  });

  it("keeps rows and the button when Load more fails, then loads on retry", async () => {
    await mount([row("a1")], {}, "cur-1");
    mocks.listActivity.mockRejectedValueOnce(new Error("x"));
    fireEvent.click(screen.getByTestId("activity-load-more"));
    expect(await screen.findByText("More could not be loaded. Try again.")).toBeTruthy();
    expect(screen.getByTestId("activity-row-a1")).toBeTruthy();
    mocks.listActivity.mockResolvedValueOnce(page([row("b1")]));
    fireEvent.click(screen.getByTestId("activity-load-more"));
    await screen.findByTestId("activity-row-b1");
    expect(mocks.listActivity.mock.calls.at(-1)?.[2]).toEqual({
      class: undefined,
      before: "cur-1",
    });
    expect(screen.queryByTestId("activity-load-more")).toBeNull();
  });

  it("re-reads on a matching coordinator.updated event only", async () => {
    await mount([row("a1")]);
    const calls = mocks.listActivity.mock.calls.length;
    act(() =>
      mocks.wsHandlers.forEach((h) =>
        h({ payload: { workspace_id: "ws-x", coordinator_id: "co-1" } }),
      ),
    );
    expect(mocks.listActivity.mock.calls.length).toBe(calls);
    mocks.listActivity.mockResolvedValue(page([row("a1"), row("a0")]));
    act(() =>
      mocks.wsHandlers.forEach((h) =>
        h({ payload: { workspace_id: "ws-1", coordinator_id: "co-1" } }),
      ),
    );
    await screen.findByTestId("activity-row-a0");
  });

  it("keeps a refusal message across Load more and clears it on a filter change", async () => {
    mocks.listActivity.mockResolvedValueOnce(page([row("a1")], "cur-1"));
    const view = render(<WhatItDid {...props()} />);
    await screen.findByTestId("activity-row-a1");
    mocks.undoActivity.mockRejectedValue(
      new ApiError("x", 409, { code: "undo_conflict", reason: "step_full" }),
    );
    fireEvent.click(screen.getByTestId("activity-undo-a1"));
    fireEvent.click(await screen.findByTestId("activity-undo-confirm"));
    await within(rowEl("a1")).findByText("The step it came from is full.");
    mocks.listActivity.mockResolvedValueOnce(page([row("b1")]));
    fireEvent.click(screen.getByTestId("activity-load-more"));
    await screen.findByTestId("activity-row-b1");
    expect(within(rowEl("a1")).getByText("The step it came from is full.")).toBeTruthy();

    mocks.search.current = "class=move";
    mocks.listActivity.mockResolvedValue(page([row("a1")]));
    view.rerender(<WhatItDid {...props()} />);
    await screen.findByTestId("activity-row-a1");
    expect(screen.queryByTestId("activity-undo-message")).toBeNull();
  });

  it("drops a stale response after the filter changes", async () => {
    let resolveFirst: (v: unknown) => void = () => undefined;
    mocks.listActivity.mockReturnValueOnce(new Promise((r) => (resolveFirst = r)));
    const view = render(<WhatItDid {...props()} />);
    mocks.search.current = "class=stop";
    mocks.listActivity.mockResolvedValueOnce(page([row("s1", { action_class: "stop" })]));
    view.rerender(<WhatItDid {...props()} />);
    await screen.findByTestId("activity-row-s1");
    await act(async () => resolveFirst(page([row("old")])));
    expect(screen.queryByTestId("activity-row-old")).toBeNull();
  });

  it("replaces the address on a filter change and removes class for All", async () => {
    await mount([row("a1")]);
    fireEvent.click(screen.getByTestId("activity-filter"));
    fireEvent.click(await screen.findByRole("option", { name: "Move task" }));
    expect(mocks.replace).toHaveBeenCalledWith("/coordinator/queue?class=move", { scroll: false });
    cleanup();
    mocks.replace.mockReset();
    mocks.search.current = "class=move&x=1";
    await mount([row("a1")]);
    fireEvent.click(screen.getByTestId("activity-filter"));
    fireEvent.click(await screen.findByRole("option", { name: "All" }));
    expect(mocks.replace).toHaveBeenCalledWith("/coordinator/queue?x=1", { scroll: false });
  });
});
