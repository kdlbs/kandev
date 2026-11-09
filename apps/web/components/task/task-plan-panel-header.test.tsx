import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TaskPlan, TaskPlanRevision } from "@/lib/types/http";

const implementPlanMock = vi.fn().mockResolvedValue(undefined);
const { downloadMock, toastMock } = vi.hoisted(() => ({
  downloadMock: vi.fn(),
  toastMock: vi.fn(),
}));
vi.mock("@/lib/utils/file-download", () => ({ triggerFileDownload: downloadMock }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: toastMock }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/hooks/domains/kanban/use-plan-actions", () => ({
  useImplementPlanRunner: () => implementPlanMock,
}));

vi.mock("./task-plan-revisions", () => ({
  TaskPlanRevisions: () => null,
}));

vi.mock("./chat/implement-plan-button", () => ({
  ImplementPlanButton: ({
    onClick,
    disabled,
  }: {
    onClick: (fresh: boolean) => void;
    disabled?: boolean;
  }) => (
    <button
      type="button"
      data-testid="implement-button"
      disabled={disabled}
      onClick={() => onClick(false)}
    >
      Implement
    </button>
  ),
}));

import { PlanPanelHeader } from "./task-plan-panel-header";

const TASK_1 = "task-1";
const REVISIONS: TaskPlanRevision[] = [];
const DEFAULT_DRAFT = "draft that will be sent";
const DOWNLOAD_LABEL = "task:download";
const SAVED_PLAN = { task_id: TASK_1, content: "saved content", title: "Plan" } as TaskPlan;

afterEach(() => {
  cleanup();
  implementPlanMock.mockClear();
  downloadMock.mockReset();
  toastMock.mockClear();
});

function header(
  attemptSave: (content: string, title?: string) => Promise<TaskPlan | null>,
  draftContent = DEFAULT_DRAFT,
  activeSessionId: string | null = "session-1",
  plan: TaskPlan | null = null,
) {
  return (
    <PlanPanelHeader
      taskId={TASK_1}
      plan={plan}
      draftContent={draftContent}
      hasUnsavedChanges={true}
      activeSessionId={activeSessionId}
      revisions={REVISIONS}
      isLoadingRevisions={false}
      isSaving={false}
      isAgentBusy={false}
      attemptSave={attemptSave}
      onOpenRevisions={() => {}}
      onRevert={async () => null}
      loadRevisionContent={async () => ""}
      previewRevisionId={null}
      setPreviewRevision={() => {}}
      comparePair={[null, null]}
      toggleCompareSelection={() => {}}
      clearComparePair={() => {}}
    />
  );
}

function renderHeader(attemptSave: (content: string, title?: string) => Promise<TaskPlan | null>) {
  return render(header(attemptSave));
}

it("downloads the current draft without a session, save, or implementation", () => {
  const attemptSave = vi.fn().mockResolvedValue(null);
  const draft = "# Olá 🌍\n\nDraft line\n";
  const view = render(header(attemptSave, draft, null, SAVED_PLAN));

  fireEvent.click(screen.getByRole("button", { name: DOWNLOAD_LABEL }));
  expect(downloadMock).toHaveBeenCalledWith({
    fileName: "plan.md",
    content: draft,
    isBinary: false,
  });
  expect(attemptSave).not.toHaveBeenCalled();
  expect(implementPlanMock).not.toHaveBeenCalled();
  const nextDraft = "Updated\n雪";
  view.rerender(header(attemptSave, nextDraft, null, SAVED_PLAN));
  fireEvent.click(screen.getByRole("button", { name: DOWNLOAD_LABEL }));
  expect(downloadMock).toHaveBeenLastCalledWith({
    fileName: "plan.md",
    content: nextDraft,
    isBinary: false,
  });
});

it.each(["", " \n\t "])("disables download for an empty or whitespace draft", (draft) => {
  render(header(vi.fn(), draft, null));
  expect(screen.getByRole("button", { name: DOWNLOAD_LABEL })).toHaveProperty("disabled", true);
});

it("shows localized feedback when the download helper throws", () => {
  downloadMock.mockImplementation(() => {
    throw new Error("failure");
  });
  const attemptSave = vi.fn();
  render(header(attemptSave, "draft", null));
  fireEvent.click(screen.getByRole("button", { name: DOWNLOAD_LABEL }));
  expect(toastMock).toHaveBeenCalledWith({ title: "task:failedToDownloadFile" });
  expect(attemptSave).not.toHaveBeenCalled();
  expect(implementPlanMock).not.toHaveBeenCalled();
});

describe("PlanPanelHeader Implement routes through the guarded save entry point", () => {
  it("calls the attemptSave prop, not a separate raw save path, before implementing", async () => {
    // Regression test: handleImplement used to call a raw savePlan prop that
    // bypassed usePlanDraft's attemptSave wrapper, so an oversized draft
    // rejected via the Implement button was never recorded in the autosave
    // suppression ref — the very next autosave tick resubmitted the same
    // rejected content. Routing through attemptSave (whatever the caller
    // passes as this prop) closes that gap; this test pins that the panel's
    // wiring passes the guarded callback and that a rejection here does not
    // proceed to implement.
    const attemptSave = vi.fn().mockResolvedValue(null); // simulates a size rejection
    renderHeader(attemptSave);

    fireEvent.click(screen.getByTestId("implement-button"));

    await waitFor(() => expect(attemptSave).toHaveBeenCalledWith(DEFAULT_DRAFT, undefined));
    expect(implementPlanMock).not.toHaveBeenCalled();
  });

  it("proceeds to implement once the guarded save succeeds", async () => {
    const savedPlan: TaskPlan = {
      task_id: TASK_1,
      content: DEFAULT_DRAFT,
      title: "Plan",
    } as TaskPlan;
    const attemptSave = vi.fn().mockResolvedValue(savedPlan);
    renderHeader(attemptSave);

    fireEvent.click(screen.getByTestId("implement-button"));

    await waitFor(() => expect(implementPlanMock).toHaveBeenCalledWith(false));
    expect(attemptSave).toHaveBeenCalledWith(DEFAULT_DRAFT, undefined);
  });
});
