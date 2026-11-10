import { Suspense, startTransition, useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  Parent,
  transport,
  openEdit,
  change,
  save,
  cancel,
  rawOld,
  focusDraft,
  assertDraft,
  expectOriginal,
  openCreate,
  dialog,
  query,
  rawNew,
  workspace,
  choose,
  Providers,
  store,
  watch,
} from "./linear-watch-dialog-lifetime.test-helpers";

// jsdom cannot mount Monaco. Preserve the real prompt wrapper/loader exports;
// only its unrelated renderer is adapted. The edited query input is shipped code.
vi.mock("@monaco-editor/react", async (original) => ({
  ...(await original<typeof import("@monaco-editor/react")>()),
  default: () => null,
}));
import { LinearIssueWatchDialog } from "./linear-issue-watch-dialog";
import { useLinearIssueWatches } from "@/hooks/domains/linear/use-linear-issue-watches";
import type { LinearIssueWatch } from "@/lib/types/linear";
const originalRowId = "linear-watch-row-watch-a";
const successToast = "Watcher updated";
const otherWorkspace = "workspace-b";
let active: ReturnType<typeof transport>;
afterEach(async () => {
  try {
    await active?.dispose();
  } finally {
    cleanup();
    vi.unstubAllGlobals();
  }
});
it.each(["reopened", "owned"] as const)(
  "accepted original update preserves %s completion ownership",
  async (schedule) => {
    active = transport();
    render(<Parent />);
    await openEdit();
    change(rawOld);
    save();
    expect(active.writes).toHaveLength(1);
    expectOriginal(active);
    let field: HTMLInputElement | undefined;
    if (schedule === "reopened") {
      cancel();
      await openEdit();
      field = focusDraft();
    }
    await active.settle(0);
    expect(screen.getByTestId(originalRowId).textContent).toContain(rawOld.trim());
    expect(screen.getByText(successToast)).toBeTruthy();
    if (field) assertDraft(field);
    else await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  },
);

// @covers AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.1, AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.2
// @covers AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.3
for (const replacement of ["same", "different", "create"] as const) {
  for (const success of [true, false]) {
    it(`old update ${success ? "success" : "failure"} preserves ${replacement} form and next Save/Cancel`, async () => {
      active = transport();
      render(<Parent />);
      await openEdit();
      change(rawOld);
      save();
      expectOriginal(active);
      cancel();
      if (replacement === "create") await openCreate();
      else await openEdit(replacement === "same" ? "watch-a" : "watch-b");
      const field = focusDraft();
      await active.settle(0, success);
      assertDraft(field);
      expect(screen.getByTestId(originalRowId).textContent).toContain(
        success ? rawOld.trim() : "initial issue query",
      );
      expect(screen.getByText(success ? successToast : /rejected-save-0/)).toBeTruthy();
      expect(
        within(dialog()).getByRole("button", {
          name: replacement === "create" ? "Create" : "Update",
        }),
      ).toHaveProperty("disabled", false);
      save();
      expect(active.writes).toHaveLength(2);
      expect(active.writes[1]).toMatchObject({
        method: replacement === "create" ? "POST" : "PATCH",
        path: `/api/v1/linear/watches/issue${{ create: "", same: "/watch-a", different: "/watch-b" }[replacement]}`,
        body: { filter: { query: rawNew.trim() } },
      });
      if (replacement === "create")
        expect(active.writes[1].body).toMatchObject({
          workspaceId: workspace,
        });
      await active.settle(1);
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      await openEdit();
      cancel();
      expect(active.writes).toHaveLength(2);
      expect(screen.queryByRole("dialog")).toBeNull();
    });
  }
}
for (const success of [true, false]) {
  it(`old create ${success ? "success" : "failure"} leaves a reopened create usable`, async () => {
    active = transport();
    render(<Parent />);
    await screen.findByTestId("linear-watch-row-watch-a");
    await openCreate();
    change(rawOld);
    save();
    expect(active.writes[0]).toMatchObject({
      method: "POST",
      path: "/api/v1/linear/watches/issue",
      body: {
        workspaceId: workspace,
        filter: { query: rawOld.trim() },
      },
    });
    cancel();
    await openCreate();
    const field = focusDraft();
    await active.settle(0, success);
    assertDraft(field);
    if (success)
      expect(screen.getByTestId("linear-watch-row-created-0").textContent).toContain(rawOld.trim());
    expect(screen.getByText(success ? "Watcher created" : /rejected-save-0/)).toBeTruthy();
    cancel();
    expect(active.writes).toHaveLength(1);
  });
  it(`old ${success ? "success" : "failure"} cannot clear a newer pending save`, async () => {
    active = transport();
    render(<Parent />);
    await openEdit();
    change(rawOld);
    save();
    cancel();
    await openEdit();
    change(rawNew);
    save();
    expect(active.writes).toHaveLength(2);
    await active.settle(0, success);
    expect(query().value).toBe(rawNew);
    expect(within(dialog()).getByRole("button", { name: "Saving…" })).toHaveProperty(
      "disabled",
      true,
    );
    await active.settle(1);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
}
it.each([false, true])(
  "current create=%s failure retains raw draft and permits a successful retry",
  async (create) => {
    active = transport();
    render(<Parent />);
    if (create) {
      await screen.findByTestId(originalRowId);
      await openCreate();
    } else await openEdit();
    change(rawOld);
    save();
    await active.settle(0, false);
    expect(query().value).toBe(rawOld);
    expect(screen.getByText(/rejected-save-0/)).toBeTruthy();
    expect(
      within(dialog()).getByRole("button", { name: create ? "Create" : "Update" }),
    ).toHaveProperty("disabled", false);
    save();
    expect(active.writes[1].body).toEqual(active.writes[0].body);
    await active.settle(1);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  },
);
it.each([true, false])(
  "unmount/remount isolates form state after success=%s while preserving transport and toast",
  async (success) => {
    active = transport();
    const view = render(<Parent />);
    await openEdit();
    change(rawOld);
    save();
    view.rerender(<Parent visible={false} />);
    view.rerender(<Parent />);
    await openEdit();
    const field = focusDraft();
    await active.settle(0, success);
    assertDraft(field);
    expectOriginal(active);
    expect(screen.getByText(success ? successToast : /rejected-save-0/)).toBeTruthy();
    expect(screen.getByTestId(originalRowId).textContent).toContain(
      success ? rawOld.trim() : "initial issue query",
    );
  },
);

type Selection = { watch: LinearIssueWatch | null; workspaceId?: string; open?: boolean };
function Scoped({ watch: selected, workspaceId, open = true }: Selection) {
  const actions = useLinearIssueWatches();
  const [show, setShow] = useState(true);
  return (
    <LinearIssueWatchDialog
      open={open && show}
      watch={selected}
      workspaceId={workspaceId}
      onOpenChange={setShow}
      onCreate={actions.create}
      onUpdate={(id, req) => actions.update(id, req, selected?.workspaceId)}
    />
  );
}
function draw(selected: LinearIssueWatch) {
  return (
    <Providers>
      <Scoped watch={selected} />
    </Providers>
  );
}
it("same-ID passive hydration preserves raw draft, selection and current pending success", async () => {
  active = transport();
  const view = render(draw(watch()));
  const field = focusDraft();
  view.rerender(draw({ ...watch(), filter: { ...watch().filter, query: "passive-refresh" } }));
  assertDraft(field);
  save();
  view.rerender(draw({ ...watch(), filter: { ...watch().filter, query: "pending-refresh" } }));
  expect(query()).toBe(field);
  expect(query().value).toBe(rawNew);
  expect(within(dialog()).getByRole("button", { name: "Saving…" })).toHaveProperty(
    "disabled",
    true,
  );
  await active.settle(0);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});
it("selected watch and stored workspace replace fields and retire pending ownership", async () => {
  active = transport();
  const view = render(draw(watch()));
  change(rawOld);
  save();
  view.rerender(
    draw({
      ...watch("watch-b"),
      workspaceId: otherWorkspace,
      filter: { ...watch().filter, query: "new-selection" },
    }),
  );
  expect(query().value).toBe("new-selection");
  const field = focusDraft();
  await active.settle(0);
  assertDraft(field);
  expect(within(dialog()).getByRole("button", { name: "Update" })).toHaveProperty(
    "disabled",
    false,
  );
  view.rerender(
    draw({
      ...watch("watch-b"),
      workspaceId: "workspace-c",
      filter: { ...watch().filter, query: "new-workspace" },
    }),
  );
  expect(query().value).toBe("new-workspace");
});
it("an uncommitted suspended replacement cannot retire the visible owner", async () => {
  active = transport();
  const pending = new Promise<never>(() => {});
  function Hold(): never {
    throw pending;
  }
  const draw = (replace = false) => (
    <Providers>
      <Suspense fallback={<div>Pending replacement</div>}>
        <Scoped watch={watch(replace ? "watch-b" : "watch-a")} />
        {replace && <Hold />}
      </Suspense>
    </Providers>
  );
  const view = render(draw());
  change(rawOld);
  save();
  act(() => startTransition(() => view.rerender(draw(true))));
  expect(screen.queryByText("Pending replacement")).toBeNull();
  await active.settle(0);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});

// @covers AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.4, AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.5
it.each([true, false])(
  "explicit create workspace switch retires old completion success=%s and preserves portable draft",
  async (success) => {
    active = transport();
    render(<Parent />);
    await screen.findByTestId(originalRowId);
    await openCreate();
    change(rawOld);
    save();
    expect(active.writes[0]).toMatchObject({
      method: "POST",
      body: {
        workspaceId: workspace,
        filter: { query: rawOld.trim() },
        workflowId: "workflow-a",
        workflowStepId: "step-a",
      },
    });
    await choose("Workspace", otherWorkspace);
    expect(query().value).toBe(rawOld);

    expect(within(dialog()).getByRole("button", { name: "Create" })).toHaveProperty(
      "disabled",
      true,
    );
    await choose("Workflow", "Fixture workflow");
    await choose("Workflow Step", "Fixture step");
    const field = focusDraft();
    act(() =>
      store.setState((s) => ({ workspaces: { ...s.workspaces, activeId: "workspace-c" } })),
    );
    await active.settle(0, success);
    assertDraft(field);
    expect(within(dialog()).getByText(otherWorkspace, { exact: true })).toBeTruthy();
    expect(screen.getByText(success ? "Watcher created" : /rejected-save-0/)).toBeTruthy();
    save();
    expect(active.writes).toHaveLength(2);
    expect(active.writes[1]).toMatchObject({
      method: "POST",
      body: { workspaceId: otherWorkspace, filter: { query: rawNew.trim() } },
    });
    await active.settle(1);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await openCreate();
    cancel();
  },
  10_000,
);
it("unrelated active hydration preserves an install-wide edit and its current save", async () => {
  active = transport();
  render(<Parent />);
  await openEdit();
  const field = focusDraft();
  act(() => store.setState((s) => ({ workspaces: { ...s.workspaces, activeId: otherWorkspace } })));
  assertDraft(field);
  save();
  expect(active.writes[0].workspace).toBe(workspace);
  act(() => store.setState((s) => ({ workspaces: { ...s.workspaces, activeId: workspace } })));
  await active.settle(0);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});
it.each([true, false])(
  "create workspace switch leaves a newer Save busy after old success=%s",
  async (success) => {
    active = transport();
    render(<Parent />);
    await screen.findByTestId(originalRowId);
    await openCreate();
    change(rawOld);
    save();
    await choose("Workspace", otherWorkspace);
    await choose("Workflow", "Fixture workflow");
    await choose("Workflow Step", "Fixture step");
    change(rawNew);
    save();
    expect(active.writes).toHaveLength(2);
    await active.settle(0, success);
    expect(query().value).toBe(rawNew);
    expect(within(dialog()).getByRole("button", { name: "Saving…" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(active.writes[1].body.workspaceId).toBe(otherWorkspace);
    await active.settle(1);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  },
);

it.each([true, false])(
  "old create success=%s preserves a later selected edit and its Save",
  async (success) => {
    active = transport();
    render(<Parent />);
    await screen.findByTestId(originalRowId);
    await openCreate();
    change(rawOld);
    save();
    cancel();
    await openEdit("watch-b");
    const field = focusDraft();
    await active.settle(0, success);
    assertDraft(field);
    save();
    expect(active.writes[0]).toMatchObject({
      method: "POST",
      body: { workspaceId: workspace, filter: { query: rawOld.trim() } },
    });
    expect(active.writes[1]).toMatchObject({
      method: "PATCH",
      workspace,
      path: "/api/v1/linear/watches/issue/watch-b",
      body: { filter: { query: rawNew.trim() } },
    });
    await active.settle(1);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  },
);
it("a passive create default change preserves the chosen workspace and pending completion", async () => {
  active = transport();
  render(<Parent />);
  await screen.findByTestId(originalRowId);
  await openCreate();
  change(rawOld);
  save();
  act(() => store.setState((s) => ({ workspaces: { ...s.workspaces, activeId: otherWorkspace } })));
  expect(query().value).toBe(rawOld);
  expect(within(dialog()).getByText(workspace, { exact: true })).toBeTruthy();
  await active.settle(0);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await openCreate();
  expect(within(dialog()).getByText(otherWorkspace, { exact: true })).toBeTruthy();
  cancel();
});

// @covers AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.5
it("create scope switch preserves portable fields and clears captured scoped selections", async () => {
  active = transport();
  render(<Parent />);
  await screen.findByTestId(originalRowId);
  await openCreate();
  await choose("Team", "Engineering (ENG)");
  await choose("Creator", "Fixture creator");
  await choose("Assignee", "Me");
  for (const name of ["Todo", "Bug", "Fixture project", "Urgent"])
    fireEvent.click(await within(dialog()).findByRole("button", { name }));
  await choose("Repository", "Fixture repository");
  change(rawOld);
  const cap = within(dialog()).getByPlaceholderText("(no cap)");
  fireEvent.change(cap, { target: { value: "0007" } });
  save();
  expect(active.writes[0].body).toMatchObject({
    workspaceId: workspace,
    workflowId: "workflow-a",
    workflowStepId: "step-a",
    repositoryId: `repo-${workspace}`,
    maxInflightTasks: 7,
    filter: {
      query: rawOld.trim(),
      teamKey: "ENG",
      assigned: "me",
      priorities: [1],
      creatorId: `user-${workspace}`,
      stateIds: [`state-${workspace}`],
      labelIds: [`label-${workspace}`],
      projectIds: [`project-${workspace}`],
    },
  });
  await choose("Workspace", otherWorkspace);
  expect(query().value).toBe(rawOld);
  expect(cap).toHaveProperty("value", "0007");
  expect(within(dialog()).getByRole("button", { name: "Todo" }).getAttribute("aria-pressed")).toBe(
    "false",
  );
  await choose("Workflow", "Fixture workflow");
  await choose("Workflow Step", "Fixture step");
  await active.settle(0);
  save();
  expect(active.writes[1].body).toMatchObject({
    workspaceId: otherWorkspace,
    workflowId: "workflow-b",
    workflowStepId: "step-b",
    repositoryId: "",
    baseBranch: "",
    maxInflightTasks: 7,
    prompt: active.writes[0].body.prompt,
    agentProfileId: active.writes[0].body.agentProfileId,
    executorProfileId: active.writes[0].body.executorProfileId,
  });
  expect(active.writes[1].body.filter).toEqual({
    query: rawOld.trim(),
    teamKey: "ENG",
    assigned: "me",
    priorities: [1],
  });
  await active.settle(1);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});
