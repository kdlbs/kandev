import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { TaskContributionIcons } from "@/components/task/task-contribution-icons";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";

const listTaskPRsMock = vi.hoisted(() => vi.fn());
const getTaskCIAutomationOptionsMock = vi.hoisted(() => vi.fn());
const TASK_ID = "task-1";
const WORKSPACE_ID = "workspace-1";
const TOOLTIP_LOADING_TEST_ID = "pr-task-tooltip-loading";

vi.mock("@/lib/api/domains/github-api", () => ({
  listTaskPRs: listTaskPRsMock,
  getTaskCIAutomationOptions: getTaskCIAutomationOptionsMock,
}));

function renderWithStore(initialState: Partial<AppState>, ui: React.ReactNode) {
  return render(
    <StateProvider initialState={initialState}>
      <TooltipProvider>{ui}</TooltipProvider>
    </StateProvider>,
  );
}

beforeEach(() => {
  listTaskPRsMock.mockReset().mockReturnValue(new Promise(() => {}));
  getTaskCIAutomationOptionsMock.mockReset().mockResolvedValue(undefined);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("PRTaskIcon hover delay", () => {
  it("cancels a pending mouse summary on Escape without fetching details", async () => {
    vi.useFakeTimers();
    renderWithStore(
      { workspaces: { items: [], activeId: WORKSPACE_ID } },
      <TaskContributionIcons taskId={TASK_ID} prInfo={{ number: 7, state: "open" }} />,
    );
    const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
    fireEvent.pointerEnter(icon, { pointerType: "mouse" });
    await act(async () => vi.advanceTimersByTime(250));
    fireEvent.keyDown(document.body, { key: "Escape" });
    await act(async () => vi.advanceTimersByTime(500));
    expect(screen.queryByTestId(TOOLTIP_LOADING_TEST_ID)).toBeNull();
    expect(listTaskPRsMock).not.toHaveBeenCalled();
    expect(getTaskCIAutomationOptionsMock).not.toHaveBeenCalled();

    fireEvent.pointerLeave(icon, { pointerType: "mouse" });
    fireEvent.pointerEnter(icon, { pointerType: "mouse" });
    await act(async () => vi.advanceTimersByTime(500));
    expect(listTaskPRsMock).toHaveBeenCalledTimes(1);
  });

  // @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.26
  // @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.28
  it("does not hydrate during a brief hover and hydrates once after deliberate disclosure", async () => {
    vi.useFakeTimers();
    const response = new Promise<{ task_prs: Record<string, TaskPR[]> }>(() => {});
    listTaskPRsMock.mockReturnValue(response);
    renderWithStore(
      { workspaces: { items: [], activeId: WORKSPACE_ID } },
      <TaskContributionIcons
        taskId={TASK_ID}
        prInfo={{ number: 7, state: "open", aggregateState: "pending" }}
      />,
    );

    const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
    act(() => fireEvent.pointerEnter(icon, { pointerType: "mouse" }));
    expect(listTaskPRsMock).not.toHaveBeenCalled();

    await act(async () => {
      vi.advanceTimersByTime(499);
    });
    expect(listTaskPRsMock).not.toHaveBeenCalled();
    expect(screen.queryByTestId(TOOLTIP_LOADING_TEST_ID)).toBeNull();

    await act(async () => {
      vi.advanceTimersByTime(1);
      await Promise.resolve();
    });
    expect(listTaskPRsMock).toHaveBeenCalledTimes(1);
    expect(screen.getAllByTestId(TOOLTIP_LOADING_TEST_ID)).not.toHaveLength(0);

    act(() => fireEvent.pointerEnter(icon, { pointerType: "mouse" }));
    expect(listTaskPRsMock).toHaveBeenCalledTimes(1);
  });
});
