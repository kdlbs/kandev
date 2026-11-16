import { Suspense, startTransition } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  Parent,
  Providers,
  Consumer,
  transport,
  watch,
  store,
  workspace,
  otherWorkspace,
  rawOld,
  rawNew,
  dialog,
  query,
  change,
  save,
  cancel,
  openEdit,
  openCreate,
  workflow,
  choose,
  focusDraft,
  assertDraft,
  expectOriginal,
  setActive,
} from "./jira-watch-dialog-lifetime.test-helpers";

// Monaco cannot mount in happy-dom; JQL uses the actual textarea, not this renderer.
vi.mock("@monaco-editor/react", async (original) => ({
  ...(await original<typeof import("@monaco-editor/react")>()),
  default: () => null,
}));
const originalId = "watch-a";
const otherId = "watch-b";
const originalRow = "jira-watch-row-watch-a";
let active: ReturnType<typeof transport>;
afterEach(async () => {
  cleanup();
  await active?.dispose();
  vi.unstubAllGlobals();
});

// @covers AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.1, AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.2
for (const origin of ["update", "create"] as const) {
  const toast = origin === "create" ? "Watcher created" : "Watcher updated";
  for (const replacement of ["same", "different", "create"] as const) {
    const target = {
      same: { method: "PATCH", suffix: "/watch-a", workspace },
      different: { method: "PATCH", suffix: "/watch-b", workspace: otherWorkspace },
      create: { method: "POST", suffix: "", workspace: null },
    }[replacement];
    it.each([true, false])(
      `${origin} success=%s preserves a ${replacement} replacement and its next Save`,
      async (success) => {
        active = transport();
        render(<Parent />);
        await screen.findByTestId(originalRow);
        if (origin === "create") await openCreate();
        else await openEdit();
        change(rawOld);
        save();
        if (origin === "update") expectOriginal(active);
        else
          expect(active.writes[0]).toMatchObject({
            method: "POST",
            body: { workspaceId: workspace, jql: rawOld },
          });
        cancel();
        if (replacement === "create") await openCreate();
        else await openEdit(replacement === "same" ? originalId : otherId);
        const field = focusDraft();
        await active.settle(0, success);
        assertDraft(field);
        const id = origin === "create" ? "created-0" : originalId;
        if (success) {
          expect(store.getState().jiraIssueWatches.items.find((w) => w.id === id)?.jql).toBe(
            rawOld,
          );
          expect(screen.getByTestId(`jira-watch-row-${id}`).textContent).toContain(rawOld);
        } else {
          expect(store.getState().jiraIssueWatches.items.some((w) => w.jql === rawOld)).toBe(false);
        }
        expect(screen.getByText(success ? toast : /rejected-save-0/)).toBeTruthy();
        expect(
          within(dialog()).getByRole("button", {
            name: replacement === "create" ? "Create" : "Update",
          }),
        ).toHaveProperty("disabled", false);
        save();
        expect(active.writes).toHaveLength(2);
        expect(active.writes[1]).toMatchObject({
          method: target.method,
          path: `/api/v1/jira/watches/issue${target.suffix}`,
          workspace: target.workspace,
          body: { jql: rawNew },
        });
        await active.settle(1);
        await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
        await openEdit();
        cancel();
        expect(active.writes).toHaveLength(2);
      },
    );
  }
}

// @covers AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.3
it.each([true, false])("old success=%s cannot clear a newer Save's busy state", async (success) => {
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
it.each(["update", "create"] as const)(
  "current %s error permits retry with the same raw payload and ordinary success closes",
  async (kind) => {
    active = transport();
    render(<Parent />);
    await screen.findByTestId(originalRow);
    if (kind === "create") await openCreate();
    else await openEdit();
    change(rawOld);
    save();
    await active.settle(0, false);
    expect(query().value).toBe(rawOld);
    expect(screen.getByText(/rejected-save-0/)).toBeTruthy();
    save();
    expect(active.writes[1].body).toEqual(active.writes[0].body);
    await active.settle(1);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  },
);

// @covers AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.4
it.each([true, false])(
  "unmount/remount isolates success=%s while accepted store publication and toast survive",
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
    expect(screen.getByText(success ? "Watcher updated" : /rejected-save-0/)).toBeTruthy();
    expect(store.getState().jiraIssueWatches.items[0].jql).toBe(success ? rawOld : watch().jql);
  },
);
it("same-ID passive hydration and unrelated active workspace changes preserve draft and current success", async () => {
  active = transport();
  const draw = (jql: string) => (
    <Providers>
      <Consumer watch={{ ...watch(), jql }} />
    </Providers>
  );
  const view = render(draw(watch().jql));
  const field = focusDraft();
  view.rerender(draw("passive-refresh"));
  setActive(otherWorkspace);
  assertDraft(field);
  save();
  view.rerender(draw("pending-refresh"));
  assertDraft(field);
  expect(within(dialog()).getByRole("button", { name: "Saving…" })).toHaveProperty(
    "disabled",
    true,
  );
  await active.settle(0);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(active.writes[0]).toMatchObject({ workspace, body: { jql: rawNew } });
});
it("committed selection and stored workspace replacement seed their fields and retire the old request", async () => {
  active = transport();
  const draw = (id: string, scope = workspace) => (
    <Providers>
      <Consumer watch={{ ...watch(id, scope), jql: id }} />
    </Providers>
  );
  const view = render(draw(originalId));
  change(rawOld);
  save();
  view.rerender(draw(otherId, otherWorkspace));
  expect(query().value).toBe(otherId);
  const field = focusDraft();
  await active.settle(0);
  assertDraft(field);
  expect(within(dialog()).getByRole("button", { name: "Update" })).toHaveProperty(
    "disabled",
    false,
  );
  view.rerender(draw(otherId));
  expect(query().value).toBe(otherId);
});
it("caller-bound create workspace replacement creates a fresh owner", async () => {
  active = transport();
  const draw = (scope: string) => (
    <Providers>
      <Consumer watch={null} workspaceId={scope} />
    </Providers>
  );
  const view = render(draw(workspace));
  await workflow();
  change(rawOld);
  save();
  view.rerender(draw(otherWorkspace));
  expect(query().value).toContain("project = PROJ");
  await workflow(otherWorkspace);
  const field = focusDraft();
  await active.settle(0);
  assertDraft(field);
  save();
  expect(active.writes[1].body).toMatchObject({ workspaceId: otherWorkspace, jql: rawNew });
  await active.settle(1);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});
it("a suspended uncommitted replacement leaves the visible owner able to close", async () => {
  active = transport();
  const pending = new Promise<never>(() => {});
  function Hold(): never {
    throw pending;
  }
  const draw = (replace = false) => (
    <Providers>
      <Suspense fallback={<div>Pending replacement</div>}>
        <Consumer watch={watch(replace ? otherId : originalId)} />
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

// @covers AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.5
it.each([true, false])(
  "explicit unlocked workspace switch during create success=%s preserves raw data and retires busy",
  async (success) => {
    active = transport();
    render(<Parent />);
    await screen.findByTestId(originalRow);
    await openCreate();
    change(rawOld);
    const numbers = within(dialog()).getAllByRole("spinbutton");
    fireEvent.change(numbers[0], { target: { value: "600" } });
    fireEvent.change(numbers[1], { target: { value: "8" } });
    save();
    const original = structuredClone(active.writes[0].body);
    await choose("Workspace", otherWorkspace);
    expect(query().value).toBe(rawOld);
    expect(
      within(dialog())
        .getByText("Workflow Step", { exact: true })
        .parentElement!.querySelector("button"),
    ).toHaveProperty("disabled", true);
    setActive(workspace);
    await workflow(otherWorkspace);
    const field = focusDraft();
    await active.settle(0, success);
    assertDraft(field);
    expect(active.writes[0].body).toEqual(original);
    expect(original).toMatchObject({
      workspaceId: workspace,
      jql: rawOld,
      pollIntervalSeconds: 600,
      maxInflightTasks: 8,
    });
    expect(
      within(dialog())
        .getAllByRole("spinbutton")
        .map((n) => (n as HTMLInputElement).value),
    ).toEqual(["600", "8"]);
    expect(
      within(dialog()).getByText("Workspace", { exact: true }).parentElement!.textContent,
    ).toContain(otherWorkspace);
    save();
    expect(active.writes[1].body).toMatchObject({
      workspaceId: otherWorkspace,
      jql: rawNew,
      workflowId: "workflow-workspace-b",
      workflowStepId: "step-workspace-b",
    });
    await active.settle(1);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  },
);
it.each([true, false])(
  "explicit create workspace switch keeps a newer Save busy after old success=%s",
  async (success) => {
    active = transport();
    render(<Parent />);
    await screen.findByTestId(originalRow);
    await openCreate();
    await choose("Repository", `Repository ${workspace}`);
    change(rawOld);
    save();
    expect(active.writes[0].body).toMatchObject({
      workspaceId: workspace,
      repositoryId: `repo-${workspace}`,
      jql: rawOld,
    });
    await choose("Workspace", otherWorkspace);
    await workflow(otherWorkspace);
    change(rawNew);
    save();
    expect(active.writes[1].body).toMatchObject({
      workspaceId: otherWorkspace,
      repositoryId: "",
      baseBranch: "",
      jql: rawNew,
    });
    await active.settle(0, success);
    expect(query().value).toBe(rawNew);
    expect(within(dialog()).getByRole("button", { name: "Saving…" })).toHaveProperty(
      "disabled",
      true,
    );
    await active.settle(1);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  },
);
it("first passive default fills an empty workspace without replacing raw input or later explicit selection", async () => {
  active = transport();
  render(
    <Providers active="">
      <JiraCreate />
    </Providers>,
  );
  const field = focusDraft();
  setActive(workspace);
  assertDraft(field);
  expect(
    within(dialog()).getByText("Workspace", { exact: true }).parentElement!.textContent,
  ).toContain(workspace);
  await choose("Workspace", otherWorkspace);
  field.focus();
  field.setSelectionRange(2, 6);
  setActive(otherWorkspace);
  setActive(workspace);
  assertDraft(field);
  expect(
    within(dialog()).getByText("Workspace", { exact: true }).parentElement!.textContent,
  ).toContain(otherWorkspace);
  await workflow(otherWorkspace);
  save();
  expect(active.writes[0].body).toMatchObject({ workspaceId: otherWorkspace, jql: rawNew });
  await active.settle(0);
});
function JiraCreate() {
  return <Consumer watch={null} />;
}
it("reselecting the current create workspace preserves workflow and pending ownership", async () => {
  active = transport();
  render(<Parent />);
  await screen.findByTestId(originalRow);
  await openCreate();
  change(rawOld);
  save();
  await choose("Workspace", workspace);
  expect(within(dialog()).getByRole("button", { name: "Saving…" })).toHaveProperty(
    "disabled",
    true,
  );
  await active.settle(0);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});

// @covers AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.6
it("JQL Test keeps its raw query and selected workspace; Cancel sends no write", async () => {
  active = transport();
  render(<Parent />);
  await openEdit(otherId);
  change(rawOld);
  fireEvent.click(within(dialog()).getByRole("button", { name: "Test JQL" }));
  await screen.findByText("Matched 0 tickets in this page.");
  expect(active.searches).toHaveLength(1);
  expect(active.searches[0]).toEqual({ workspace: otherWorkspace, body: rawOld });
  cancel();
  expect(active.writes).toHaveLength(0);
});
