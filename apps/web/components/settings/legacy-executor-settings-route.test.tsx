import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { StateProvider } from "@/components/state-provider";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import { LegacyExecutorSettingsRoute } from "./legacy-executor-settings-route";

const { replace } = vi.hoisted(() => ({ replace: vi.fn() }));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ replace }),
}));

vi.mock("@/app/settings/executor/[id]/page", () => ({
  default: ({ executorId }: { executorId: string }) => (
    <div data-testid="legacy-executor">{executorId}</div>
  ),
}));

vi.mock("@/app/settings/executor/[id]/profile/[profileId]/page", () => ({
  default: ({ profileId }: { profileId: string }) => (
    <div data-testid="legacy-profile">{profileId}</div>
  ),
}));

const TIMESTAMP = "2026-08-24T00:00:00Z";

afterEach(() => {
  cleanup();
  replace.mockReset();
});

function executor(type: Executor["type"]): Executor {
  const id = "executor/primary";
  const profile: ExecutorProfile = {
    id: "profile/primary",
    executor_id: id,
    executor_type: type,
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
    type,
    status: "active",
    is_system: false,
    config: {},
    profiles: [profile],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

function renderRoute(record: Executor, profileId?: string) {
  render(
    <StateProvider
      initialState={{
        executors: { items: [record] },
        auth: {
          mode: "enabled",
          authenticated: true,
          user: {
            id: "member-1",
            email: "member@example.com",
            display_name: "Member",
            role: "member",
            status: "active",
          },
        },
      }}
    >
      <LegacyExecutorSettingsRoute executorId={record.id} profileId={profileId} />
    </StateProvider>,
  );
}

describe("LegacyExecutorSettingsRoute", () => {
  it("redirects a member's bookmarked Kubernetes executor before mounting legacy controls", async () => {
    renderRoute(executor("k8s"));

    await waitFor(() =>
      expect(replace).toHaveBeenCalledWith("/settings/executors/profile%2Fprimary"),
    );
    expect(screen.queryByTestId("legacy-executor")).toBeNull();
  });

  it("keeps an orphaned Kubernetes executor on the connection recovery page", async () => {
    const orphan = { ...executor("k8s"), profiles: [] };
    renderRoute(orphan);

    await waitFor(() =>
      expect(replace).toHaveBeenCalledWith("/settings/executors/k8s/executor%2Fprimary"),
    );
    expect(screen.queryByTestId("legacy-executor")).toBeNull();
  });

  it("redirects a bookmarked Kubernetes profile to the Kubernetes-aware editor", async () => {
    renderRoute(executor("k8s"), "profile/primary");

    await waitFor(() =>
      expect(replace).toHaveBeenCalledWith("/settings/executors/profile%2Fprimary"),
    );
    expect(screen.queryByTestId("legacy-profile")).toBeNull();
  });

  it("does not redirect a Kubernetes executor to a profile owned by another executor", () => {
    renderRoute(executor("k8s"), "profile/from-another-executor");

    expect(screen.getByTestId("legacy-profile")).toBeTruthy();
    expect(replace).not.toHaveBeenCalled();
  });

  it.each([
    { profileId: undefined, testId: "legacy-executor" },
    { profileId: "profile/primary", testId: "legacy-profile" },
  ])("keeps SSH on its legacy scoped page", ({ profileId, testId }) => {
    renderRoute(executor("ssh"), profileId);

    expect(screen.getByTestId(testId)).toBeTruthy();
    expect(replace).not.toHaveBeenCalled();
  });
});
