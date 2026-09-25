import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import ExecutorsHubPage from "@/app/settings/executors/page";
import { StateProvider } from "@/components/state-provider";
import { ExecutorEnvironmentDisclosure } from "@/components/task/executor-environment-disclosure";
import type { TaskEnvironment } from "@/lib/api/domains/task-environment-api";
import type { Executor, ExecutorProfile } from "@/lib/types/http";

const { push } = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push }),
}));

vi.mock("@/components/task/executor-environment-info", () => ({
  EnvironmentInfo: ({ kubernetesActions }: { kubernetesActions?: ReactNode }) => (
    <>{kubernetesActions}</>
  ),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

const TIMESTAMP = "2026-08-24T00:00:00Z";

afterEach(() => {
  cleanup();
  push.mockReset();
});

function profile(executorId: string): ExecutorProfile {
  return {
    id: "profile/primary",
    executor_id: executorId,
    executor_type: "local_docker",
    name: "Primary profile",
    config: {},
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

function executor(): Executor {
  const id = "executor/primary";
  return {
    id,
    name: "Primary executor",
    type: "local_docker",
    status: "active",
    is_system: false,
    config: {},
    profiles: [profile(id)],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

const kubernetesEnvironment = {
  id: "environment-1",
  task_id: "task-1",
  repository_id: "repository-1",
  executor_type: "k8s",
  executor_id: "executor/primary",
  executor_profile_id: "profile/primary",
  agent_execution_id: "execution-1",
  control_port: 0,
  status: "running",
  created_at: TIMESTAMP,
} satisfies TaskEnvironment;

describe("executor profile navigation", () => {
  it("opens the hub profile card in the encoded canonical editor", () => {
    const record = executor();
    render(
      <StateProvider initialState={{ executors: { items: [record] } }}>
        <ExecutorsHubPage />
      </StateProvider>,
    );

    fireEvent.click(screen.getByText("Primary profile"));

    expect(push).toHaveBeenCalledWith("/settings/executors/profile%2Fprimary");
  });

  it("opens the task disclosure settings action in the encoded canonical editor", () => {
    render(
      <ExecutorEnvironmentDisclosure
        env={kubernetesEnvironment}
        container={null}
        ssh={null}
        kubernetes={null}
        kubernetesLoaded={false}
        kubernetesError={null}
        loading={false}
        refreshing={false}
        isResetting={false}
        onRefresh={async () => {}}
        onReset={vi.fn()}
      />,
    );

    expect(screen.getByTestId("executor-settings-link").getAttribute("href")).toBe(
      "/settings/executors/profile%2Fprimary",
    );
  });
});
