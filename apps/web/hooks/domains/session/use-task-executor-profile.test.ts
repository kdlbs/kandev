import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Executor } from "@/lib/types/http";
import { useTaskExecutorProfile } from "./use-task-executor-profile";

const mockFetchTaskEnvironment = vi.fn();

const EXECUTOR_ID = "executor-1";
const EXECUTOR_NAME = "Local Exec";
const PROFILE_ID = "profile-1";
const PROFILE_NAME = "Profile 1";
const TASK_ID = "task-1";
const TASK_ENV_ID = "env-1";
const REPO_ID = "repo-1";
const AGENT_EXEC_ID = "agent-exec-1";
const CONTROL_PORT = 8080;
const TIMESTAMP = "2026-01-01T00:00:00Z";
const PROFILE_TEMPLATE = {
  id: PROFILE_ID,
  executor_id: EXECUTOR_ID,
  name: PROFILE_NAME,
  prepare_script: "",
  cleanup_script: "",
  created_at: TIMESTAMP,
  updated_at: TIMESTAMP,
};
const EXECUTOR_TEMPLATE = {
  id: EXECUTOR_ID,
  name: EXECUTOR_NAME,
  type: "local_pc",
  status: "idle",
  is_system: false,
  profiles: [
    {
      ...PROFILE_TEMPLATE,
      executor_type: "local_pc",
      executor_name: EXECUTOR_NAME,
    },
  ],
  created_at: TIMESTAMP,
  updated_at: TIMESTAMP,
} satisfies Executor;

let mockExecutors: Executor[] = [
  {
    ...EXECUTOR_TEMPLATE,
  },
];

const BASE_ENV = {
  id: TASK_ENV_ID,
  task_id: TASK_ID,
  repository_id: REPO_ID,
  executor_type: "local_pc",
  executor_id: EXECUTOR_ID,
  executor_profile_id: PROFILE_ID,
  agent_execution_id: AGENT_EXEC_ID,
  control_port: CONTROL_PORT,
  status: "ready",
  created_at: TIMESTAMP,
  updated_at: TIMESTAMP,
};

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: () => ({ executors: mockExecutors }),
}));

vi.mock("@/lib/api/domains/task-environment-api", () => ({
  fetchTaskEnvironment: (...args: Parameters<typeof mockFetchTaskEnvironment>) =>
    mockFetchTaskEnvironment(...args),
}));

describe("useTaskExecutorProfile", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetchTaskEnvironment.mockReset();
    mockFetchTaskEnvironment.mockResolvedValue(BASE_ENV);
    mockExecutors = [
      {
        ...EXECUTOR_TEMPLATE,
      },
    ];
  });

  it("resolves matching executor profile and fills fallback executor metadata", async () => {
    const { result } = renderHook(() => useTaskExecutorProfile(TASK_ID));

    await waitFor(() => {
      expect(mockFetchTaskEnvironment).toHaveBeenCalledWith(TASK_ID);
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(result.current).toMatchObject({
      id: PROFILE_ID,
      name: PROFILE_NAME,
      executor_type: "local_pc",
      executor_name: EXECUTOR_NAME,
    });
  });

  it("returns null when environment profile cannot be found", async () => {
    mockFetchTaskEnvironment.mockResolvedValue({
      ...BASE_ENV,
      executor_profile_id: "missing-profile",
    });
    const { result } = renderHook(() => useTaskExecutorProfile(TASK_ID));

    await act(async () => {
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(result.current).toBeNull();
    });
  });

  it("returns null when disabled", async () => {
    const { result } = renderHook(() => useTaskExecutorProfile(TASK_ID, false));

    expect(mockFetchTaskEnvironment).not.toHaveBeenCalled();
    expect(result.current).toBeNull();
  });

  it("returns null when task ID is missing", async () => {
    const { result } = renderHook(() => useTaskExecutorProfile(""));

    expect(mockFetchTaskEnvironment).not.toHaveBeenCalled();
    expect(result.current).toBeNull();
  });

  it("does not re-fetch when only unrelated executor metadata changes", async () => {
    const { rerender } = renderHook(() => useTaskExecutorProfile(TASK_ID));

    await waitFor(() => {
      expect(mockFetchTaskEnvironment).toHaveBeenCalledTimes(1);
    });

    mockExecutors = [
      {
        ...mockExecutors[0],
        status: "running",
      },
    ];
    rerender();

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockFetchTaskEnvironment).toHaveBeenCalledTimes(1);
  });
});
