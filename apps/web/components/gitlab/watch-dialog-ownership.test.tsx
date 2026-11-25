import { Suspense, startTransition } from "react";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GitLabWatchDialog } from "./watch-dialog";
import { useGitLabReviewWatches } from "@/hooks/domains/gitlab/use-gitlab-review-watches";
import { useGitLabIssueWatches } from "@/hooks/domains/gitlab/use-gitlab-issue-watches";
import {
  Parent,
  Providers,
  change,
  close,
  dialog,
  items,
  openCreate,
  openEdit,
  query,
  rawNew,
  rawOld,
  row,
  save,
  transport,
  workspace,
  type Kind,
  type Watch,
} from "./watch-dialog-ownership.test-helpers";

// jsdom cannot create Monaco. Preserve its real loader exports and ScriptEditor
// wrapper; only the unrelated prompt renderer is replaced. Query inputs are real.
vi.mock("@monaco-editor/react", async (original) => ({
  ...(await original<typeof import("@monaco-editor/react")>()),
  default: () => null,
}));
let active: ReturnType<typeof transport>;
afterEach(async () => {
  await active?.dispose();
  cleanup();
  vi.unstubAllGlobals();
});
function fixture() {
  active = transport();
  return active;
}
function focusDraft() {
  change(rawNew);
  const field = query();
  field.focus();
  field.setSelectionRange(4, 9);
  return field;
}
function assertDraft(field: HTMLInputElement) {
  expect(query()).toBe(field);
  expect(field.value).toBe(rawNew);
  expect(document.activeElement).toBe(field);
  expect([field.selectionStart, field.selectionEnd]).toEqual([4, 9]);
}
function expectRequest(f: ReturnType<typeof fixture>, kind: Kind, create: boolean, index = 0) {
  const write = f.writes[index];
  expect({ method: write.method, path: write.path, workspace: write.workspace }).toEqual({
    method: create ? "POST" : "PUT",
    path: `/api/v1/gitlab/watches/${kind}${create ? "" : "/watch-a"}`,
    workspace,
  });
  let prompt = "Investigate {{issue.url}}";
  if (create)
    prompt =
      kind === "review"
        ? "Review GitLab merge request {{mr.url}}. Summarize risks and leave actionable feedback."
        : "Investigate GitLab issue {{issue.url}} and implement the requested change.";
  expect(write.body).toEqual({
    ...(create ? { workspace_id: workspace } : { max_inflight_tasks: 0 }),
    custom_query: rawOld.trim(),
    workflow_id: "workflow-a",
    workflow_step_id: "step-a",
    projects: create ? [] : [{ path: "group/watch-a" }],
    repository_id: "",
    base_branch: "",
    agent_profile_id: "agent-profile-a",
    executor_profile_id: "executor-profile-a",
    prompt,
    poll_interval_seconds: 300,
    cleanup_policy: "auto",
    ...(kind === "review" ? { review_scope: "user" } : { labels: create ? [] : ["bug"] }),
  });
}
const rejected = "rejected-save-0";
function defineReopeningCases(kind: Kind) {
  for (const create of [false, true]) {
    for (const replacement of ["same", "different", "create"] as const) {
      for (const success of [true, false]) {
        it(`old ${create ? "create" : "update"} ${success ? "ACK" : "rejection"} preserves ${replacement} opening and next Save`, async () => {
          const f = fixture();
          render(<Parent />);
          if (create) await openCreate(kind);
          else await openEdit(kind);
          change(rawOld);
          save();
          await waitFor(() => expect(f.writes).toHaveLength(1));
          expectRequest(f, kind, create);
          close();
          if (replacement === "create") await openCreate(kind);
          else await openEdit(kind, replacement === "same" ? "watch-a" : "watch-b");
          const field = focusDraft();
          await f.settle(0, success);
          if (success) {
            expect(
              items(kind).find((w) => w.id === (create ? "created-0" : "watch-a"))?.custom_query,
            ).toBe(rawOld.trim());
            expect(
              screen.getByText(
                `${kind === "review" ? "Review" : "Issue"} watch ${create ? "created" : "updated"}`,
              ),
            ).toBeTruthy();
          } else {
            expect(screen.getByText(rejected)).toBeTruthy();
            expect(within(dialog()).queryByText(rejected)).toBeNull();
            expect(items(kind).find((w) => w.id === "watch-a")?.custom_query).toBe("state=opened");
          }
          assertDraft(field);
          const next = within(dialog()).getByRole("button", {
            name: /^(Update|Create) watch$/,
          }) as HTMLButtonElement;
          expect(next.disabled).toBe(false);
          save();
          await waitFor(() => expect(f.writes).toHaveLength(2));
          expect(f.writes[1].body.custom_query).toBe(rawNew.trim());
          expect(f.writes[1].path).toBe(
            `/api/v1/gitlab/watches/${kind}${{ create: "", same: "/watch-a", different: "/watch-b" }[replacement]}`,
          );
          await f.settle(1);
          await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
        });
      }
    }
  }
}
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.20, AC-INTEGRATIONS-GITLAB-INTEGRATION-001.21
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.22
for (const kind of ["review", "issue"] as const) {
  describe(`${kind} actual settings parent`, () => {
    it("current opening success publishes the original watch and closes", async () => {
      const f = fixture();
      render(<Parent />);
      await openEdit(kind);
      change(rawOld);
      save();
      await waitFor(() => expect(f.writes).toHaveLength(1));
      expectRequest(f, kind, false);
      await f.settle(0);
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(items(kind).find((w) => w.id === "watch-a")?.custom_query).toBe(rawOld.trim());
      expect(
        screen.getByText(kind === "review" ? "Review watch updated" : "Issue watch updated"),
      ).toBeTruthy();
    });
    defineReopeningCases(kind);
    it("old settlement cannot release a new opening's active Save", async () => {
      const f = fixture();
      render(<Parent />);
      await openEdit(kind);
      change(rawOld);
      save();
      close("Close");
      await openEdit(kind);
      change(rawNew);
      save();
      await waitFor(() => expect(f.writes).toHaveLength(2));
      await f.settle(0);
      expect(
        (within(dialog()).getByRole("button", { name: "Saving..." }) as HTMLButtonElement).disabled,
      ).toBe(true);
      await f.settle(1);
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(items(kind).find((w) => w.id === "watch-a")?.custom_query).toBe(rawNew.trim());
    });
    it("current rejection preserves the draft and permits retry then Cancel", async () => {
      const f = fixture();
      render(<Parent />);
      await openEdit(kind);
      change(rawOld);
      save();
      await f.settle(0, false);
      expect(within(dialog()).getByText(rejected)).toBeTruthy();
      expect(query().value).toBe(rawOld);
      change(rawNew);
      save();
      await waitFor(() => expect(f.writes).toHaveLength(2));
      expect(within(dialog()).queryByText(rejected)).toBeNull();
      await f.settle(1, false);
      close();
      await openEdit(kind);
      expect(query().value).toBe("state=opened");
      close();
    });
    it("unmounted parent still publishes accepted original writes without closing a remount", async () => {
      const f = fixture();
      const view = render(<Parent />);
      await openEdit(kind);
      change(rawOld);
      save();
      view.rerender(<Parent visible={false} />);
      view.rerender(<Parent />);
      await openEdit(kind);
      const field = focusDraft();
      await f.settle(0);
      expect(items(kind).find((w) => w.id === "watch-a")?.custom_query).toBe(rawOld.trim());
      assertDraft(field);
      close();
    });
  });
}

type Selection = { kind: Kind; workspaceId: string; watch: Watch | null; open?: boolean };
function Scoped({
  kind,
  workspaceId,
  watch,
  open = true,
  changed,
}: Selection & { changed: (open: boolean) => void }) {
  const review = useGitLabReviewWatches(workspaceId);
  const issue = useGitLabIssueWatches(workspaceId);
  const actions = kind === "review" ? review : issue;
  return (
    <GitLabWatchDialog
      kind={kind}
      workspaceId={workspaceId}
      watch={watch}
      open={open}
      onOpenChange={changed}
      onCreate={(request) =>
        actions.create(
          request as Parameters<typeof review.create>[0] & Parameters<typeof issue.create>[0],
        )
      }
      onUpdate={(id, request) => actions.update(id, request, workspaceId)}
    />
  );
}
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.21, AC-INTEGRATIONS-GITLAB-INTEGRATION-001.23
for (const replacement of ["workspace", "kind", "watch", "create"] as const) {
  it(`committed ${replacement} replacement owns state while original scoped write is accepted`, async () => {
    const f = fixture();
    const changed = vi.fn();
    const original: Selection = { kind: "review", workspaceId: workspace, watch: row() };
    const draw = (selection: Selection) => (
      <Providers>
        <Scoped {...selection} changed={changed} />
      </Providers>
    );
    const view = render(draw(original));
    change(rawOld);
    save();
    const selection: Selection = { ...original };
    if (replacement === "workspace") {
      selection.workspaceId = "watch-workspace-b";
      selection.watch = row("watch-a", selection.workspaceId);
    }
    if (replacement === "kind") selection.kind = "issue";
    if (replacement === "watch") selection.watch = row("watch-b");
    if (replacement === "create") selection.watch = null;
    view.rerender(draw(selection));
    const field = focusDraft();
    await f.settle(0);
    expect(changed).not.toHaveBeenCalled();
    assertDraft(field);
    expectRequest(f, "review", false);
    expect(
      items("review").find((w) => w.workspace_id === workspace && w.id === "watch-a")?.custom_query,
    ).toBe(rawOld.trim());
  });
}
it("same-ID hydration preserves raw input and pending ownership, while initial defaults and later selection initialize", async () => {
  const f = fixture();
  const changed = vi.fn();
  const original: Selection = { kind: "review", workspaceId: workspace, watch: null, open: false };
  const draw = (selection: Selection) => (
    <Providers>
      <Scoped {...selection} changed={changed} />
    </Providers>
  );
  const view = render(draw(original));
  expect(screen.queryByRole("dialog")).toBeNull();
  view.rerender(draw({ ...original, open: true }));
  expect(query().value).toBe("");
  expect(
    (within(dialog()).getByLabelText("Poll interval (seconds)") as HTMLInputElement).value,
  ).toBe("300");
  view.rerender(draw({ ...original, watch: row(), open: true }));
  expect(query().value).toBe("state=opened");
  const field = focusDraft();
  view.rerender(
    draw({ ...original, watch: { ...row(), custom_query: "server-refresh" }, open: true }),
  );
  assertDraft(field);
  change(rawOld);
  save();
  view.rerender(
    draw({ ...original, watch: { ...row(), custom_query: "second-refresh" }, open: true }),
  );
  expect(query().value).toBe(rawOld);
  await f.settle(0);
  expect(changed).toHaveBeenCalledExactlyOnceWith(false);
});

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.23
it("an uncommitted suspended scope render cannot retire the current opening", async () => {
  const f = fixture();
  const changed = vi.fn();
  const suspended = new Promise<never>(() => {});
  function Hold(): never {
    throw suspended;
  }
  const draw = (replace = false) => (
    <Providers>
      <Suspense fallback={<div>Pending replacement</div>}>
        <Scoped
          kind={replace ? "issue" : "review"}
          workspaceId={workspace}
          watch={row()}
          changed={changed}
        />
        {replace && <Hold />}
      </Suspense>
    </Providers>
  );
  const view = render(draw());
  change(rawOld);
  save();
  act(() => {
    startTransition(() => view.rerender(draw(true)));
  });
  expect(screen.queryByText("Pending replacement")).toBeNull();
  expect(dialog().querySelector("#review-watch-query")).toBeTruthy();
  await f.settle(0);
  expect(changed).toHaveBeenCalledExactlyOnceWith(false);
});
