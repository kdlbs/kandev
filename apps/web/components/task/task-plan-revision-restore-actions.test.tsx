import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { TaskPlanRevision } from "@/lib/types/http";
import { TaskPlanRevisions } from "./task-plan-revisions";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: true, isFinePointer: false }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ tasks: { activeSessionId: null }, taskSessions: { items: {} } }),
}));
vi.mock("@/lib/toast/sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/components/agent-logo", () => ({ AgentLogo: () => null }));
vi.mock("./task-plan-preview-dialog", () => ({ PlanRevisionPreviewDialog: () => null }));
vi.mock("./task-plan-diff-dialog", () => ({ PlanRevisionDiffDialog: () => null }));
afterEach(cleanup);

const revisions: TaskPlanRevision[] = [2, 1].map((version) => ({
  id: `revision-${version}`,
  task_id: "task-1",
  revision_number: version,
  title: `Version ${version}`,
  content: `Plan ${version}`,
  author_kind: "user",
  author_name: "user",
  created_at: "2026-09-10T10:00:00Z",
  updated_at: "2026-09-10T10:00:00Z",
}));

function history() {
  const props = {
    taskId: "task-1",
    revisions,
    isLoading: false,
    isSaving: false,
    onOpen: vi.fn(),
    onRevert: vi.fn().mockResolvedValue(revisions[0]),
    loadRevisionContent: vi.fn().mockResolvedValue("content"),
    previewRevisionId: null,
    setPreviewRevision: vi.fn(),
    comparePair: [null, null] as [string | null, string | null],
    toggleCompareSelection: vi.fn(),
    clearComparePair: vi.fn(),
  };
  const view = render(<TaskPlanRevisions {...props} />);
  const trigger = screen.getByTestId("plan-rewind-button");
  const open = () => {
    fireEvent.click(trigger);
    fireEvent.click(screen.getByTestId("plan-revision-revert-button"));
  };
  open();
  return { ...view, props, trigger, open };
}

it("hands off plan history to a standalone phone sheet and restores trigger focus", async () => {
  const { props, trigger, open } = history();
  const sheet = await screen.findByRole("dialog", { name: "Restore to version 1?" });
  expect(sheet.getAttribute("data-slot")).toBe("drawer-content");
  expect(screen.queryByTestId("plan-revisions-popover")).toBeNull();
  expect(screen.queryByTestId("plan-revision-restore-inline-confirmation")).toBeNull();
  fireEvent.click(within(sheet).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(props.onRevert).not.toHaveBeenCalled();
  open();
  const confirm = await screen.findByTestId("plan-revision-restore-confirm");
  fireEvent.click(confirm);
  fireEvent.click(confirm);
  await waitFor(() => expect(props.onRevert).toHaveBeenCalledExactlyOnceWith("revision-1"));
});

it("cancels a captured revision when the task changes", async () => {
  const { props, rerender } = history();
  const sheet = await screen.findByRole("dialog", { name: "Restore to version 1?" });
  const confirm = within(sheet).getByTestId("plan-revision-restore-confirm");
  rerender(<TaskPlanRevisions {...props} taskId="task-2" />);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  fireEvent.click(confirm);
  expect(props.onRevert).not.toHaveBeenCalled();
});
