import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { TaskPlanRevision } from "@/lib/types/http";

import { TaskPlanRevisions } from "./task-plan-revisions";

const mocks = vi.hoisted(() => ({
  breakpoint: { isFinePointer: true },
  toast: { success: vi.fn(), error: vi.fn() },
}));

const restorePopoverTestId = "plan-revision-restore-confirm-popover";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => mocks.breakpoint,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ tasks: { activeSessionId: null }, taskSessions: { items: {} } }),
}));

vi.mock("@/lib/toast/sonner", () => ({ toast: mocks.toast }));
vi.mock("@/components/agent-logo", () => ({ AgentLogo: () => null }));
vi.mock("./task-plan-preview-dialog", () => ({ PlanRevisionPreviewDialog: () => null }));
vi.mock("./task-plan-diff-dialog", () => ({ PlanRevisionDiffDialog: () => null }));

function revision(revisionNumber: number): TaskPlanRevision {
  return {
    id: `revision-${revisionNumber}`,
    task_id: "task-1",
    revision_number: revisionNumber,
    title: `Version ${revisionNumber}`,
    content: `Plan ${revisionNumber}`,
    author_kind: "user",
    author_name: "user",
    created_at: "2026-08-20T10:00:00.000Z",
    updated_at: "2026-08-20T10:00:00.000Z",
  };
}

function renderHistory(onRevert = vi.fn().mockResolvedValue(revision(3))) {
  render(
    <TaskPlanRevisions
      taskId="task-1"
      revisions={[revision(2), revision(1)]}
      isLoading={false}
      isSaving={false}
      onOpen={vi.fn()}
      onRevert={onRevert}
      loadRevisionContent={vi.fn().mockResolvedValue("content")}
      previewRevisionId={null}
      setPreviewRevision={vi.fn()}
      comparePair={[null, null]}
      toggleCompareSelection={vi.fn()}
      clearComparePair={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByTestId("plan-rewind-button"));
}

function olderRevisionRow() {
  return screen
    .getAllByTestId("plan-revision-row")
    .find((row) => row.getAttribute("data-revision-number") === "1") as HTMLElement;
}

describe("TaskPlanRevisions restore confirmation", () => {
  beforeEach(() => {
    mocks.breakpoint.isFinePointer = true;
    mocks.toast.success.mockReset();
    mocks.toast.error.mockReset();
  });

  afterEach(cleanup);

  it("anchors desktop confirmation to the revision action and preserves cancel", async () => {
    const onRevert = vi.fn().mockResolvedValue(revision(3));
    renderHistory(onRevert);

    const row = olderRevisionRow();
    const restore = within(row).getByTestId("plan-revision-revert-button");
    fireEvent.click(restore);

    const confirmation = await screen.findByTestId(restorePopoverTestId);
    expect(confirmation.textContent).toContain("Restore to version 1?");
    expect(screen.queryByTestId("plan-revert-confirm-dialog")).toBeNull();

    fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByTestId(restorePopoverTestId)).toBeNull());
    expect(onRevert).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(restore);
  });

  it("confirms the named desktop revision exactly once", async () => {
    const onRevert = vi.fn().mockResolvedValue(revision(3));
    renderHistory(onRevert);

    const row = olderRevisionRow();
    fireEvent.click(within(row).getByTestId("plan-revision-revert-button"));
    const confirmation = await screen.findByTestId(restorePopoverTestId);
    fireEvent.click(within(confirmation).getByTestId("plan-revision-restore-confirm"));

    await waitFor(() => expect(onRevert).toHaveBeenCalledTimes(1));
    expect(onRevert).toHaveBeenCalledWith("revision-1");
    expect(mocks.toast.success).toHaveBeenCalledWith("Plan restored to v1");
  });

  it("morphs the phone row into 44px inline actions without an overlay", async () => {
    mocks.breakpoint.isFinePointer = false;
    renderHistory();

    const row = olderRevisionRow();
    fireEvent.click(within(row).getByTestId("plan-revision-revert-button"));

    const confirmation = await screen.findByTestId("plan-revision-restore-inline-confirmation");
    expect(confirmation.textContent).toContain("v1");
    expect(screen.queryByTestId(restorePopoverTestId)).toBeNull();
    expect(within(confirmation).getByRole("button", { name: "Restore v1" }).className).toContain(
      "h-11",
    );
    expect(within(confirmation).getByRole("button", { name: "Restore v1" }).className).toContain(
      "min-w-11",
    );
  });
});
