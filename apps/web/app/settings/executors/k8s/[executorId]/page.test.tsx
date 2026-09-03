import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { StateProvider } from "@/components/state-provider";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import KubernetesExecutorPage from "./page";

const { replace, useKubernetesExecutorResource } = vi.hoisted(() => ({
  replace: vi.fn(),
  useKubernetesExecutorResource: vi.fn(),
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ replace, push: vi.fn() }),
}));

vi.mock("@/hooks/domains/settings/use-kubernetes-settings", () => ({
  useKubernetesExecutorResource,
}));

const TIMESTAMP = "2026-08-31T00:00:00Z";

function kubernetesExecutor(withProfile: boolean): Executor {
  const id = "executor/primary";
  const profile: ExecutorProfile = {
    id: "profile/primary",
    executor_id: id,
    executor_type: "k8s",
    name: "Primary profile",
    config: {},
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
  return {
    id,
    name: "Primary executor",
    type: "k8s",
    status: "active",
    is_system: false,
    config: {},
    profiles: withProfile ? [profile] : [],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

function renderPage(record: Executor) {
  render(
    <StateProvider initialState={{ executors: { items: [record] } }}>
      <KubernetesExecutorPage executorId={record.id} />
    </StateProvider>,
  );
}

afterEach(() => {
  cleanup();
  replace.mockReset();
  useKubernetesExecutorResource.mockReset();
});

describe("KubernetesExecutorPage", () => {
  it("redirects a configured executor to its unified profile settings page", async () => {
    renderPage(kubernetesExecutor(true));

    await waitFor(() =>
      expect(replace).toHaveBeenCalledWith("/settings/executors/profile%2Fprimary"),
    );
    expect(useKubernetesExecutorResource).not.toHaveBeenCalled();
  });

  it("keeps the standalone connection page for an executor without profiles", () => {
    useKubernetesExecutorResource.mockReturnValue({ loading: true });

    renderPage(kubernetesExecutor(false));

    expect(screen.getByText("Loading executor...")).toBeTruthy();
    expect(replace).not.toHaveBeenCalled();
    expect(useKubernetesExecutorResource).toHaveBeenCalledWith("executor/primary");
  });
});
