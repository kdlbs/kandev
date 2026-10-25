import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Executor } from "@/lib/types/http";
import {
  createDefaultKubernetesProfileConfig,
  parseKubernetesExecutorConfig,
} from "./kubernetes-config";
import { KubernetesProfileClusterSection } from "./kubernetes-profile-cluster-section";

const mocks = vi.hoisted(() => ({
  runDiagnostics: vi.fn(async () => null),
  refreshSessions: vi.fn(async () => []),
  useSessions: vi.fn(),
}));

vi.mock("@/hooks/domains/settings/use-kubernetes-settings", () => ({
  useKubernetesDiagnostics: () => ({
    testing: false,
    result: null,
    error: null,
    run: mocks.runDiagnostics,
  }),
  useKubernetesSessions: (executorId: string) => {
    mocks.useSessions(executorId);
    return {
      sessions: [],
      loading: false,
      error: null,
      refresh: mocks.refreshSessions,
    };
  },
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

const EXECUTOR = {
  id: "executor/one",
  name: "Remote production",
  type: "k8s",
  status: "active",
  is_system: false,
  config: {
    auth_mode: "kubeconfig",
    kube_context: "prod-eu",
    namespace: "kandev-workloads",
    request_timeout_seconds: "30",
  },
  created_at: "2026-08-31T10:00:00Z",
  updated_at: "2026-08-31T10:00:00Z",
} satisfies Executor;

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("KubernetesProfileClusterSection", () => {
  it("leads with the inline connection editor and tests both unsaved drafts", () => {
    const form = {
      ...createDefaultKubernetesProfileConfig(),
      mainContainer: "draft-agent",
      workspaceMode: "empty_dir" as const,
    };
    const connection = {
      ...parseKubernetesExecutorConfig(EXECUTOR.name, EXECUTOR.config),
      namespace: "draft-namespace",
    };
    const onConnectionChange = vi.fn();
    render(
      <KubernetesProfileClusterSection
        executor={EXECUTOR}
        form={form}
        connectionForm={connection}
        connectionBaseline={parseKubernetesExecutorConfig(EXECUTOR.name, EXECUTOR.config)}
        onConnectionChange={onConnectionChange}
        canManage
      />,
    );

    expect(screen.getByRole("heading", { name: "Cluster connection" })).toBeTruthy();
    expect(screen.getByTestId("kubernetes-connection-card")).toBeTruthy();
    expect((screen.getByTestId("kubernetes-namespace") as HTMLInputElement).value).toBe(
      "draft-namespace",
    );
    expect(screen.getByTestId("kubernetes-diagnostics-card")).toBeTruthy();
    expect(screen.getByTestId("kubernetes-sessions-card")).toBeTruthy();
    expect(mocks.useSessions).toHaveBeenCalledWith("executor/one");

    fireEvent.click(screen.getByRole("button", { name: "Test Kubernetes" }));
    expect(mocks.runDiagnostics).toHaveBeenCalledWith({
      config: expect.objectContaining({ namespace: "draft-namespace" }),
      profile_config: expect.objectContaining({
        main_container: "draft-agent",
        "workspace.mode": "empty_dir",
      }),
    });
    expect(screen.queryByRole("button", { name: "Edit cluster connection" })).toBeNull();
  });

  it("keeps connection access and sessions visible for members while disabling diagnostics", () => {
    render(
      <KubernetesProfileClusterSection
        executor={EXECUTOR}
        form={createDefaultKubernetesProfileConfig()}
        connectionForm={parseKubernetesExecutorConfig(EXECUTOR.name, EXECUTOR.config)}
        connectionBaseline={parseKubernetesExecutorConfig(EXECUTOR.name, EXECUTOR.config)}
        onConnectionChange={vi.fn()}
        canManage={false}
      />,
    );

    const namespace = screen.getByTestId("kubernetes-namespace") as HTMLInputElement;
    expect((namespace.closest("fieldset") as HTMLFieldSetElement).disabled).toBe(true);
    expect(screen.queryByRole("button", { name: "View cluster connection" })).toBeNull();
    expect(screen.getByRole("button", { name: "Test Kubernetes" }).hasAttribute("disabled")).toBe(
      true,
    );
    expect(screen.getByTestId("kubernetes-sessions-card")).toBeTruthy();
  });
});
