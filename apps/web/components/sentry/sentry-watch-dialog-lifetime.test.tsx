import { Suspense, startTransition, useState } from "react";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
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
  Providers,
  store,
  watch,
} from "./sentry-watch-dialog-lifetime.test-helpers";

// jsdom cannot mount Monaco. Preserve the real prompt wrapper/loader exports;
// only its unrelated renderer is adapted. The edited query input is shipped code.
vi.mock("@monaco-editor/react", async (original) => ({
  ...(await original<typeof import("@monaco-editor/react")>()),
  default: () => null,
}));
import { SentryIntegrationPage } from "./sentry-settings";
import { SentryIssueWatchDialog } from "./sentry-issue-watch-dialog";
import { useSentryIssueWatches } from "@/hooks/domains/sentry/use-sentry-issue-watches";
import type { SentryIssueWatch } from "@/lib/types/sentry";
const originalRowId = "sentry-watch-row-watch-a";
const successToast = "Watcher updated";
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

// @covers AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.1, AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.2
// @covers AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.3
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
        success ? rawOld.trim() : "is:unresolved",
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
        path: `/api/v1/sentry/watches/issue${{ create: "", same: "/watch-a", different: "/watch-b" }[replacement]}`,
        body: { filter: { query: rawNew.trim() } },
      });
      if (replacement === "create")
        expect(active.writes[1].body).toMatchObject({
          workspaceId: workspace,
          sentryInstanceId: "instance-a",
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
    await screen.findByTestId("sentry-watch-row-watch-a");
    await openCreate();
    change(rawOld);
    save();
    expect(active.writes[0]).toMatchObject({
      method: "POST",
      path: "/api/v1/sentry/watches/issue",
      body: {
        workspaceId: workspace,
        sentryInstanceId: "instance-a",
        filter: { query: rawOld.trim() },
      },
    });
    cancel();
    await openCreate();
    const field = focusDraft();
    await active.settle(0, success);
    assertDraft(field);
    if (success)
      expect(screen.getByTestId("sentry-watch-row-created-0").textContent).toContain(rawOld.trim());
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
it("current failure retains raw draft and permits a successful retry", async () => {
  active = transport();
  render(<Parent />);
  await openEdit();
  change(rawOld);
  save();
  await active.settle(0, false);
  expect(query().value).toBe(rawOld);
  expect(screen.getByText(/rejected-save-0/)).toBeTruthy();
  expect(within(dialog()).getByRole("button", { name: "Update" })).toHaveProperty(
    "disabled",
    false,
  );
  save();
  expect(active.writes[1].body).toEqual(active.writes[0].body);
  await active.settle(1);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});
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
    // A new hook instance retains its own loaded row; this is not a shared store list.
    expect(screen.getByTestId(originalRowId).textContent).not.toContain(rawOld.trim());
  },
);

// @covers AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.2, AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.4
it.each(["route", "active"] as const)(
  "actual settings page %s workspace replacement retires the old form owner",
  async (scope) => {
    active = transport();
    const draw = (workspaceId?: string) => (
      <Providers>
        <SentryIntegrationPage workspaceId={workspaceId} />
      </Providers>
    );
    const view = render(draw(scope === "route" ? workspace : undefined));
    await openEdit();
    change(rawOld);
    save();
    if (scope === "route") view.rerender(draw("workspace-b"));
    else
      act(() =>
        store.setState((s) => ({ workspaces: { ...s.workspaces, activeId: "workspace-b" } })),
      );
    await openEdit("watch-b");
    const field = focusDraft();
    await active.settle(0);
    assertDraft(field);
    expectOriginal(active);
    expect(screen.getByText(successToast)).toBeTruthy();
    save();
    expect(active.writes[1]).toMatchObject({
      workspace: "workspace-b",
      path: "/api/v1/sentry/watches/issue/watch-b",
    });
    await active.settle(1);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  },
);

type Selection = { watch: SentryIssueWatch | null; workspaceId?: string; open?: boolean };
function Scoped({ watch: selected, workspaceId = workspace, open = true }: Selection) {
  const actions = useSentryIssueWatches(workspaceId);
  const [show, setShow] = useState(true);
  return (
    <SentryIssueWatchDialog
      open={open && show}
      watch={selected}
      workspaceId={workspaceId}
      onOpenChange={setShow}
      onCreate={actions.create}
      onUpdate={actions.update}
    />
  );
}
function draw(selected: SentryIssueWatch) {
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
it("selected watch and instance identity replace fields and retire pending ownership", async () => {
  active = transport();
  const view = render(draw(watch()));
  change(rawOld);
  save();
  view.rerender(
    draw({
      ...watch("watch-b"),
      sentryInstanceId: "instance-b",
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
      sentryInstanceId: "instance-c",
      filter: { ...watch().filter, query: "new-instance" },
    }),
  );
  expect(query().value).toBe("new-instance");
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
