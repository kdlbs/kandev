import { cleanup, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { KubernetesConnectionCard } from "./kubernetes-connection-card";
import { KubernetesDiagnosticsCard } from "./kubernetes-diagnostics-card";
import { KubernetesSessionsCard } from "./kubernetes-sessions-card";
import { KubernetesWorkspaceCard } from "./kubernetes-workspace-card";
import {
  createDefaultKubernetesExecutorForm,
  createDefaultKubernetesProfileConfig,
} from "./kubernetes-config";

const responsive = vi.hoisted(() => ({ isMobile: false }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: responsive.isMobile }),
}));

afterEach(cleanup);

describe("Kubernetes settings cards", () => {
  it("hides kubeconfig-only fields for in-cluster auth and disables member edits", () => {
    const form = { ...createDefaultKubernetesExecutorForm(), authMode: "in_cluster" as const };

    render(<KubernetesConnectionCard form={form} baseline={form} onChange={vi.fn()} readOnly />);

    expect(screen.queryByLabelText("Kubeconfig path")).toBeNull();
    expect(
      (screen.getByLabelText("Fixed namespace").closest("fieldset") as HTMLFieldSetElement)
        .disabled,
    ).toBe(true);
  });

  it("shows only fields belonging to the selected workspace mode", () => {
    const baseline = createDefaultKubernetesProfileConfig();
    const { rerender } = render(
      <KubernetesWorkspaceCard
        form={{ ...baseline, workspaceMode: "empty_dir" }}
        baseline={baseline}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId("kubernetes-empty-dir-warning")).toBeTruthy();
    expect(screen.queryByLabelText("Claim size")).toBeNull();
    rerender(
      <KubernetesWorkspaceCard
        form={{ ...baseline, workspaceMode: "existing_claim", claimName: "shared" }}
        baseline={baseline}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByLabelText("Existing claim name")).toBeTruthy();
    expect(screen.queryByLabelText("Claim size")).toBeNull();
  });

  it("keeps member testing visible and disabled with an explanation", () => {
    render(
      <KubernetesDiagnosticsCard
        testing={false}
        result={null}
        error={null}
        onTest={vi.fn()}
        canManage={false}
      />,
    );

    expect(screen.getByRole("button", { name: "Test Kubernetes" }).hasAttribute("disabled")).toBe(
      true,
    );
    expect(screen.getByText("Only an administrator can run this Kubernetes test.")).toBeTruthy();
  });

  it("renders causal diagnostic steps and PodTemplate warnings", () => {
    render(
      <KubernetesDiagnosticsCard
        testing={false}
        result={{
          success: false,
          steps: [
            {
              key: "rbac",
              success: false,
              duration_ms: 12,
              detail: "Denied permission",
            },
          ],
          warnings: [{ path: "template.spec", message: "Privileged container" }],
        }}
        error={null}
        onTest={vi.fn()}
        canManage
      />,
    );

    expect(screen.getByText("Required permissions")).toBeTruthy();
    expect(screen.getByText("Denied permission")).toBeTruthy();
    expect(screen.getByText("template.spec")).toBeTruthy();
    expect(screen.getByText("Privileged container")).toBeTruthy();
  });

  it("shows complete task and session identities on mobile session cards", () => {
    responsive.isMobile = true;
    render(<KubernetesSessionsCard state={sessionsState()} />);

    const card = screen.getByTestId("kubernetes-mobile-session-list");
    expect(within(card).getByText("task-123456789")).toBeTruthy();
    expect(within(card).getByText("session-987654321")).toBeTruthy();
    expect(within(card).getByText("Task")).toBeTruthy();
    expect(within(card).getByText("Session")).toBeTruthy();
  });
});

describe("Kubernetes session status cards", () => {
  it("shows the sanitized failure reason beneath desktop session status", () => {
    responsive.isMobile = false;
    render(<KubernetesSessionsCard state={sessionsState()} />);

    const row = screen.getByText("task-123").closest("tr");
    expect(row).not.toBeNull();
    const statusCell = within(row as HTMLTableRowElement).getByTestId("kubernetes-session-status");
    expect(within(statusCell).getByText("pods is forbidden: RBAC denied")).toBeTruthy();
  });

  it("labels a successfully completed Pod as succeeded instead of terminated", () => {
    responsive.isMobile = false;
    const state = sessionsState();
    state.sessions[0] = {
      ...state.sessions[0],
      pod_phase: "Succeeded",
      container_state: "terminated",
      failure_reason: undefined,
    };

    render(<KubernetesSessionsCard state={state} />);

    expect(screen.getByText("Succeeded")).toBeTruthy();
    expect(screen.queryByText("Terminated")).toBeNull();
  });
});

type KubernetesSessionsState = ComponentProps<typeof KubernetesSessionsCard>["state"];

function sessionsState(): KubernetesSessionsState {
  return {
    sessions: [
      {
        task_id: "task-123456789",
        session_id: "session-987654321",
        pod_name: "kandev-session-1",
        pod_phase: "Failed",
        container_state: "terminated",
        restarts: 2,
        workspace_kind: "managed_pvc",
        created_at: "2026-08-24T10:00:00Z",
        failure_reason: "pods is forbidden: RBAC denied",
      },
    ],
    loading: false,
    error: null,
    refresh: vi.fn(async () => []),
  };
}
