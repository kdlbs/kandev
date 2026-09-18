import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { useEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import { LegacyExecutorSettingsRoute } from "./legacy-executor-settings-route";

const LEGACY_EXECUTOR_TESTID = "legacy-executor";
const { replace } = vi.hoisted(() => ({ replace: vi.fn() }));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ replace }),
}));

vi.mock("@/app/settings/executor/[id]/page", () => ({
  default: ({ executorId }: { executorId: string }) => (
    <div data-testid={LEGACY_EXECUTOR_TESTID}>{executorId}</div>
  ),
}));

const TIMESTAMP = "2026-08-24T00:00:00Z";
const PROFILE_ID = "profile/primary";
const CANONICAL_PROFILE_PATH = "/settings/executors/profile%2Fprimary";
const PROFILE_UNAVAILABLE_TESTID = "executor-profile-unavailable";

afterEach(() => {
  cleanup();
  replace.mockReset();
  window.history.replaceState({}, "", "/settings");
});

function executor(type: Executor["type"]): Executor {
  const id = "executor/primary";
  const profile: ExecutorProfile = {
    id: PROFILE_ID,
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

function renderRoute(
  record: Executor | null,
  profileId?: string,
  executorId?: string,
  options: { executorsLoaded?: boolean } = {},
) {
  render(
    <StateProvider
      initialState={{
        executors: { items: record ? [record] : [] },
        settingsData: { executorsLoaded: options.executorsLoaded ?? true, agentsLoaded: true },
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
      <LegacyExecutorSettingsRoute
        executorId={executorId ?? record?.id ?? "executor/missing"}
        profileId={profileId}
      />
    </StateProvider>,
  );
}

function HydrateExecutors({ record }: { record: Executor }) {
  const store = useAppStoreApi();
  useEffect(() => {
    store.getState().setExecutors([record]);
    store.getState().setSettingsData({ executorsLoaded: true });
  }, [record, store]);
  return null;
}

it("waits for executor hydration before rendering an unavailable profile state", () => {
  renderRoute(null, PROFILE_ID, "executor/missing", { executorsLoaded: false });

  expect(screen.queryByTestId(PROFILE_UNAVAILABLE_TESTID)).toBeNull();
  expect(replace).not.toHaveBeenCalled();
});

describe("LegacyExecutorSettingsRoute", () => {
  it("redirects a member's bookmarked Kubernetes executor before mounting legacy controls", async () => {
    renderRoute(executor("k8s"));

    await waitFor(() => expect(replace).toHaveBeenCalledWith(CANONICAL_PROFILE_PATH));
    expect(screen.queryByTestId(LEGACY_EXECUTOR_TESTID)).toBeNull();
  });

  it("keeps an orphaned Kubernetes executor on the connection recovery page", async () => {
    const orphan = { ...executor("k8s"), profiles: [] };
    renderRoute(orphan);

    await waitFor(() =>
      expect(replace).toHaveBeenCalledWith("/settings/executors/k8s/executor%2Fprimary"),
    );
    expect(screen.queryByTestId(LEGACY_EXECUTOR_TESTID)).toBeNull();
  });

  it("redirects a bookmarked Kubernetes profile to the Kubernetes-aware editor", async () => {
    renderRoute(executor("k8s"), PROFILE_ID);

    await waitFor(() => expect(replace).toHaveBeenCalledWith(CANONICAL_PROFILE_PATH));
    expect(screen.queryByTestId("legacy-profile")).toBeNull();
  });

  it("does not redirect a Kubernetes executor to a profile owned by another executor", () => {
    renderRoute(executor("k8s"), "profile/from-another-executor");

    expect(screen.getByTestId(PROFILE_UNAVAILABLE_TESTID)).toBeTruthy();
    expect(replace).not.toHaveBeenCalled();
  });

  it.each([
    "local",
    "local_docker",
    "ssh",
    "sprites",
    "local_pc",
    "worktree",
    "remote_docker",
  ] as const)("redirects a %s profile bookmark to the canonical editor", async (type) => {
    renderRoute(executor(type), PROFILE_ID);

    await waitFor(() => expect(replace).toHaveBeenCalledWith(CANONICAL_PROFILE_PATH));
    expect(screen.queryByTestId(LEGACY_EXECUTOR_TESTID)).toBeNull();
  });

  it("keeps an executor-only SSH route on the connection editor", () => {
    renderRoute(executor("ssh"));

    expect(screen.getByTestId(LEGACY_EXECUTOR_TESTID)).toBeTruthy();
    expect(replace).not.toHaveBeenCalled();
  });

  it("renders an unavailable state for a missing profile without choosing another profile", () => {
    const record = executor("local_docker");
    record.profiles = [
      {
        ...record.profiles![0],
        id: "profile/other",
      },
    ];

    renderRoute(record, PROFILE_ID);

    expect(screen.getByTestId(PROFILE_UNAVAILABLE_TESTID)).toBeTruthy();
    expect(replace).not.toHaveBeenCalled();
  });

  it("renders an unavailable state for a missing executor", () => {
    renderRoute(null, PROFILE_ID, "executor/missing");

    expect(screen.getByTestId(PROFILE_UNAVAILABLE_TESTID)).toBeTruthy();
    expect(replace).not.toHaveBeenCalled();
  });

  it("preserves query parameters and fragments while replacing a valid bookmark", async () => {
    window.history.replaceState(
      {},
      "",
      "/settings/executor/executor%2Fprimary/profile/profile%2Fprimary?tab=advanced#docker",
    );
    renderRoute(executor("ssh"), PROFILE_ID);

    await waitFor(() =>
      expect(replace).toHaveBeenCalledWith(
        "/settings/executors/profile%2Fprimary?tab=advanced#docker",
      ),
    );
  });

  it("retries a valid bookmark when executor state hydrates after the route mounts", async () => {
    const record = executor("ssh");
    render(
      <StateProvider
        initialState={{
          executors: { items: [] },
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
        <LegacyExecutorSettingsRoute executorId={record.id} profileId={PROFILE_ID} />
        <HydrateExecutors record={record} />
      </StateProvider>,
    );

    await waitFor(() => expect(replace).toHaveBeenCalledWith(CANONICAL_PROFILE_PATH));
  });
});
