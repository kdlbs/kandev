import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { StateProvider } from "@/components/state-provider";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import { ExecutorProfilesCard } from "./executor-profiles-card";

const { push } = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push }) }));

const TIMESTAMP = "2026-08-24T00:00:00Z";

afterEach(() => {
  cleanup();
  push.mockReset();
});

function profile(executorId: string, executorType: Executor["type"]): ExecutorProfile {
  return {
    id: "profile/primary",
    executor_id: executorId,
    executor_type: executorType,
    name: "Primary profile",
    config: {},
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

function executor(type: Executor["type"]): Executor {
  const id = "executor/primary";
  return {
    id,
    name: "Primary executor",
    type,
    status: "active",
    is_system: false,
    config: {},
    profiles: [profile(id, type)],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

function renderCard(record: Executor) {
  render(
    <StateProvider initialState={{ executors: { items: [record] } }}>
      <ExecutorProfilesCard executorId={record.id} profiles={record.profiles ?? []} />
    </StateProvider>,
  );
}

describe("ExecutorProfilesCard profile navigation", () => {
  it("opens a Kubernetes profile in the Kubernetes-aware editor", () => {
    renderCard(executor("k8s"));

    fireEvent.click(screen.getByText("Primary profile"));

    expect(push).toHaveBeenCalledWith("/settings/executors/profile%2Fprimary");
  });

  it("preserves the scoped legacy profile route for SSH", () => {
    renderCard(executor("ssh"));

    fireEvent.click(screen.getByText("Primary profile"));

    expect(push).toHaveBeenCalledWith(
      "/settings/executor/executor%2Fprimary/profile/profile%2Fprimary",
    );
  });
});
