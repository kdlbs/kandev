import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { TaskEnvironment } from "@/lib/api/domains/task-environment-api";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";
import { EnvironmentInfo } from "./executor-environment-info";

const ENVIRONMENT: TaskEnvironment = {
  id: "environment-1",
  task_id: "task-1",
  repository_id: "repository-1",
  executor_type: "k8s",
  executor_id: "executor-1",
  executor_profile_id: "profile-1",
  agent_execution_id: "execution-1",
  control_port: 8765,
  status: "ready",
  workspace_path: "/workspace",
  created_at: "2026-08-25T09:30:00Z",
};

const SESSION: KubernetesSession = {
  session_id: "session-1",
  task_id: "task-1",
  pod_name: "kandev-pod-1",
  pod_phase: "Running",
  container_state: "running",
  restarts: 0,
  workspace_kind: "empty_dir",
  created_at: "2026-08-25T09:31:00Z",
};

afterEach(cleanup);

describe("EnvironmentInfo Kubernetes details", () => {
  it("renders exact Pod details instead of the empty-resource fallback", () => {
    const KubernetesEnvironmentInfo = EnvironmentInfo as unknown as React.ComponentType<{
      env: TaskEnvironment;
      container: null;
      ssh: null;
      kubernetes: KubernetesSession;
      kubernetesLoaded: boolean;
      kubernetesError: string | null;
      loading: boolean;
    }>;

    render(
      <KubernetesEnvironmentInfo
        env={ENVIRONMENT}
        container={null}
        ssh={null}
        kubernetes={SESSION}
        kubernetesLoaded
        kubernetesError={null}
        loading={false}
      />,
    );

    expect(screen.getByTestId("kubernetes-environment-summary")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Kubernetes Pod" })).toBeTruthy();
    expect(screen.getByText("kandev-pod-1")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Copy Pod" })).toBeTruthy();
    expect(screen.getByTestId("kubernetes-restart-count").textContent).toBe("0");
    expect(screen.getAllByText("Running")).toHaveLength(2);
    expect(screen.getByText("running")).toBeTruthy();
    expect(screen.getByText("empty_dir")).toBeTruthy();
    expect(screen.queryByText("No resource details available.")).toBeNull();
  });
});
