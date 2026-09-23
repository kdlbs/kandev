import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { useChangeWorkflow } from "@/hooks/domains/kanban/use-change-workflow";
import { ChangeWorkflowDialog } from "./change-workflow-dialog";

const CURRENT_AGENT_LABEL = "Current agent";
const CURRENT_PROFILE_ID = "profile-current";

const { useChangeWorkflowMock, responsiveMock } = vi.hoisted(() => ({
  useChangeWorkflowMock: vi.fn(),
  responsiveMock: { isMobile: false },
}));

vi.mock("@/hooks/domains/kanban/use-change-workflow", () => ({
  useChangeWorkflow: useChangeWorkflowMock,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsiveMock,
}));

vi.mock("@/components/task/workflow-move-preview-footer", () => ({
  WorkflowMovePreviewFooter: () => <div data-testid="workflow-entry-preview" />,
}));

type HookState = ReturnType<typeof useChangeWorkflow>;

function makeHookState(): HookState {
  const task = {
    id: "task-1",
    title: "Keep the thread",
    workflow_id: "source-workflow",
    workflow_step_id: "source-step",
    updated_at: "2026-09-22T12:00:00Z",
    workflow_agent_overrides: {
      workflow_id: "source-workflow",
      steps: [{ source_profile_id: "profile-old", replacement_profile_id: "profile-retired" }],
    },
  } as unknown as HookState["task"];
  const step = (
    id: string,
    name: string,
    sessionTarget?: { kind: "initial" } | { kind: "step"; step_id: string },
  ) => ({
    id,
    name,
    position: id === "build" ? 0 : 1,
    color: "#abcdef",
    session_target: sessionTarget,
  });
  return {
    task,
    taskStatus: "success",
    taskError: null,
    workflows: [],
    workflowsStatus: "success",
    workflowsError: null,
    destinations: [{ id: "destination", name: "Product", workspace_id: "workspace-1" }],
    selectedWorkflowId: "destination",
    selectedWorkflow: { id: "destination", name: "Product", workspace_id: "workspace-1" },
    changeWorkflow: vi.fn(),
    snapshot: {
      workflow: { id: "destination", name: "Product" },
      steps: [
        step("build", "Build"),
        step("review", "Review", { kind: "step", step_id: "build" }),
        step("wrap-up", "Wrap-up", { kind: "initial" }),
      ],
      tasks: [],
    } as unknown as HookState["snapshot"],
    snapshotStatus: "success",
    snapshotError: null,
    retrySnapshot: vi.fn(),
    profiles: [],
    profilesStatus: "success",
    profilesError: null,
    profileOptions: [
      {
        value: CURRENT_PROFILE_ID,
        label: CURRENT_AGENT_LABEL,
        renderLabel: () => CURRENT_AGENT_LABEL,
      },
    ],
    rows: [
      {
        sourceProfileId: CURRENT_PROFILE_ID,
        sourceLabel: CURRENT_AGENT_LABEL,
        sourceAgentName: "agent",
        stepIds: ["build", "review"],
        stepNames: ["Build", "Review"],
        replacementProfileId: "profile-retired",
        replacementLabel: "Retired agent",
        replacementAvailable: false,
      },
    ],
    setOverride: vi.fn(),
    overrides: {},
    selectedStepId: "build",
    setSelectedStepId: vi.fn(),
    sourceWorkflowName: "Backlog",
    sourceStepName: "Todo",
    sourceChanged: false,
    uncertainResult: false,
    submitError: null,
    sourceProfileErrorId: undefined,
    isSubmitting: false,
    canSubmit: false,
    submit: vi.fn(),
    retryTask: vi.fn(),
    retryWorkflows: vi.fn(),
    retryProfiles: vi.fn(),
    workflowChange: {
      expected_workflow_id: "source-workflow",
      expected_step_id: "source-step",
      expected_updated_at: "2026-09-22T12:00:00Z",
      agent_overrides: {},
    },
  } as unknown as HookState;
}

afterEach(() => {
  cleanup();
});

beforeEach(() => {
  responsiveMock.isMobile = false;
  useChangeWorkflowMock.mockReset();
  useChangeWorkflowMock.mockReturnValue(makeHookState());
});

describe("ChangeWorkflowDialog", () => {
  it("shows the destination form, task-scoped agent mapping, and conversation relationships", () => {
    render(
      <ChangeWorkflowDialog
        open
        onOpenChange={vi.fn()}
        taskId="task-1"
        workspaceId="workspace-1"
      />,
    );

    expect(screen.getByRole("dialog", { name: "Change workflow..." })).toBeTruthy();
    expect(screen.getByText("Backlog · Todo")).toBeTruthy();
    expect(screen.getByTestId("change-workflow-destination")).toBeTruthy();
    expect(screen.getByTestId("change-workflow-step")).toBeTruthy();
    expect(
      screen.getByText(
        "Changing workflows replaces this task's existing workflow agent overrides.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Review uses the conversation from Build.")).toBeTruthy();
    expect(screen.getByText("Wrap-up uses the initial conversation.")).toBeTruthy();
    expect(screen.getByTestId("workflow-entry-preview")).toBeTruthy();
    expect(screen.getByRole("alert").textContent).toContain("This profile is unavailable");
    expect((screen.getByTestId("change-workflow-submit") as HTMLButtonElement).disabled).toBe(true);
  });

  it("resets a replacement to the workflow profile and returns cancel to the caller", () => {
    const state = makeHookState();
    useChangeWorkflowMock.mockReturnValue(state);
    const onOpenChange = vi.fn();
    render(
      <ChangeWorkflowDialog
        open
        onOpenChange={onOpenChange}
        taskId="task-1"
        workspaceId="workspace-1"
      />,
    );

    fireEvent.click(screen.getByTestId(`change-workflow-profile-reset-${CURRENT_PROFILE_ID}`));
    expect(state.setOverride).toHaveBeenCalledWith(CURRENT_PROFILE_ID, CURRENT_PROFILE_ID);
    fireEvent.click(screen.getByTestId("change-workflow-cancel"));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("uses a full-height phone surface with one scroll area and safe-area action space", () => {
    responsiveMock.isMobile = true;
    render(
      <ChangeWorkflowDialog
        open
        onOpenChange={vi.fn()}
        taskId="task-1"
        workspaceId="workspace-1"
      />,
    );

    const drawer = screen.getByTestId("change-workflow-drawer");
    const body = screen.getByTestId("change-workflow-scroll");
    const footer = screen.getByTestId("change-workflow-submit").parentElement;
    expect(drawer.className).toContain("100dvh");
    expect(body.className).toContain("overflow-y-auto");
    expect(footer?.className).toContain("safe-area-inset-bottom");
    expect(screen.getByTestId("change-workflow-submit").className).toContain("min-h-12");
  });
});
