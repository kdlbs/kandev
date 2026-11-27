import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { WorkspaceRecoveryProjection } from "@/lib/types/http";
import { WorkspaceRecoveryProgress } from "./workspace-recovery-progress";

const mocks = vi.hoisted(() => ({
  state: {
    repositories: {
      itemsByWorkspaceId: {
        workspace: [{ id: "repository-1", name: "landing" }],
      },
    },
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, unknown>) =>
      values ? `${key} ${Object.values(values).join(" ")}` : key,
  }),
}));

function projection(overrides: Partial<WorkspaceRecoveryProjection> = {}) {
  return {
    task_id: "task-1",
    environment_id: "environment-1",
    session_id: "session-1",
    operation_id: "operation-1",
    attempt_id: "attempt-1",
    ownership_generation: "generation-1",
    revision: "2",
    kind: "managed_clone_relocation",
    state: "running",
    phase: "restoring",
    repository_id: "repository-1",
    repository_position: 2,
    repository_total: 2,
    completed_slots: 1,
    workspace_complete: false,
    agent_ready: false,
    runner_live: true,
    started_at: "2026-10-05T12:00:00Z",
    updated_at: "2026-10-05T12:01:00Z",
    ...overrides,
  } satisfies WorkspaceRecoveryProjection;
}

describe("WorkspaceRecoveryProgress", () => {
  it("announces the repository and phase without live-announcing heartbeat time", () => {
    render(<WorkspaceRecoveryProgress projection={projection()} repositoryName="landing" />);

    expect(
      screen.getByTestId("workspace-recovery-progress").getAttribute("data-recovery-phase"),
    ).toBe("restoring");
    expect(screen.getByText("task:workspaceRecoveryRepositoryPosition 2 2 landing")).toBeTruthy();
    const announcement = screen.getByRole("status");
    expect(announcement.textContent).toContain("task:workspaceRecoveryPhaseRestoring");
    expect(announcement.textContent).not.toContain("task:workspaceRecoveryLastUpdate");
    expect(screen.getByTestId("workspace-recovery-last-update")).toBeTruthy();
    expect(screen.getByText("task:workspaceRecoveryCopiesRetained")).toBeTruthy();
  });

  it("keeps migrated files distinct from agent readiness and handles unknown phases", () => {
    render(
      <WorkspaceRecoveryProgress
        projection={projection({ phase: "future_phase", workspace_complete: true })}
      />,
    );

    expect(screen.getByText("task:workspaceRecoveryFilesMovedTitle")).toBeTruthy();
    expect(screen.getByText("task:workspaceRecoveryPhaseUnknown")).toBeTruthy();
    expect(screen.getByTestId("workspace-recovery-agent-pending")).toBeTruthy();
  });

  it("offers an explicit check when status remains unresolved without a projection", () => {
    const onCheckStatus = vi.fn();
    render(<WorkspaceRecoveryProgress statusCheck="unresolved" onCheckStatus={onCheckStatus} />);

    fireEvent.click(screen.getByTestId("workspace-recovery-check-status"));
    expect(onCheckStatus).toHaveBeenCalledOnce();
    expect(screen.getByText("task:workspaceRecoveryStatusUnresolved")).toBeTruthy();
  });

  it("retires the progress panel after the agent is ready", () => {
    const { container } = render(
      <WorkspaceRecoveryProgress
        projection={projection({ state: "completed", phase: "resuming", agent_ready: true })}
      />,
    );

    expect(container.childNodes).toHaveLength(0);
  });
});
