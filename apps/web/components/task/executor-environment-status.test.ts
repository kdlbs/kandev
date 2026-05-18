import { describe, expect, it } from "vitest";
import {
  getEnvironmentStatusSnapshot,
  resolveExecutorEnvironmentStatus,
} from "./executor-environment-status";
import type { ContainerLiveStatus, TaskEnvironment } from "@/lib/api/domains/task-environment-api";

const baseEnv: TaskEnvironment = {
  id: "env-1",
  task_id: "task-1",
  repository_id: "repo-1",
  executor_type: "local_docker",
  executor_id: "local-docker",
  executor_profile_id: "profile-1",
  agent_execution_id: "exec-1",
  control_port: 41001,
  status: "ready",
  created_at: "2026-05-02T00:00:00Z",
  updated_at: "2026-05-02T00:00:00Z",
};

function container(overrides: Partial<ContainerLiveStatus>): ContainerLiveStatus {
  return {
    container_id: "container-1",
    state: "running",
    status: "running",
    started_at: "2026-05-02T00:00:00Z",
    ...overrides,
  };
}

describe("resolveExecutorEnvironmentStatus", () => {
  it("uses live container state ahead of recorded environment status", () => {
    expect(
      resolveExecutorEnvironmentStatus(
        { ...baseEnv, status: "ready" },
        container({ state: "exited", status: "exited", exit_code: 137 }),
      ),
    ).toEqual({ label: "exited (137)", tone: "error" });
  });

  it("shows paused containers as a warning state", () => {
    expect(resolveExecutorEnvironmentStatus(baseEnv, container({ state: "paused" }))).toEqual({
      label: "paused",
      tone: "warn",
    });
  });

  it("falls back to the persisted environment status when no live container exists", () => {
    expect(resolveExecutorEnvironmentStatus({ ...baseEnv, status: "creating" }, null)).toEqual({
      label: "starting",
      tone: "warn",
    });
  });
});

describe("getEnvironmentStatusSnapshot", () => {
  it("uses a stable key for status transition notifications", () => {
    expect(getEnvironmentStatusSnapshot(baseEnv, container({ state: "running" }))).toEqual({
      key: "running:running",
      label: "running",
      tone: "running",
    });
    expect(getEnvironmentStatusSnapshot(null, null)).toEqual({
      key: "none",
      label: "not created",
      tone: "neutral",
    });
  });
});
