import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Executor } from "@/lib/types/http";

const {
  createExecutor,
  createExecutorProfile,
  deleteExecutor,
  getKubernetesSessionImpact,
  listKubernetesSessions,
  testKubernetesConnection,
} = vi.hoisted(() => ({
  createExecutor: vi.fn(),
  createExecutorProfile: vi.fn(),
  deleteExecutor: vi.fn(),
  getKubernetesSessionImpact: vi.fn(),
  listKubernetesSessions: vi.fn(),
  testKubernetesConnection: vi.fn(),
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  createExecutor,
  createExecutorProfile,
  deleteExecutor,
  fetchExecutor: vi.fn(),
  updateExecutor: vi.fn(),
}));

vi.mock("@/lib/api/domains/kubernetes-api", () => ({
  getKubernetesSessionImpact,
  listKubernetesSessions,
  testKubernetesConnection,
}));

import {
  createKubernetesExecutorPair,
  KubernetesExecutorRollbackRejectedError,
  kubernetesExecutorSettingsPath,
  upsertExecutor,
  useKubernetesDiagnostics,
  useKubernetesSessionImpact,
  useKubernetesSessions,
} from "./use-kubernetes-settings";

const EXECUTOR_ID = "executor-1";
const TIMESTAMP = "2026-08-24T00:00:00Z";

function executor(id: string, name = id): Executor {
  return {
    id,
    name,
    type: "k8s",
    status: "active",
    is_system: false,
    config: {},
    profiles: [],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

beforeEach(() => {
  createExecutor.mockReset();
  createExecutorProfile.mockReset();
  deleteExecutor.mockReset();
  getKubernetesSessionImpact.mockReset();
  listKubernetesSessions.mockReset();
  testKubernetesConnection.mockReset();
});

describe("Kubernetes executor create orchestration", () => {
  it("returns both created identities and builds the canonical connection route", async () => {
    const createdExecutor = {
      id: "executor/primary",
      name: "Production cluster",
      type: "k8s",
      config: { namespace: "agents" },
    };
    const createdProfile = profile("profile-1", createdExecutor.id);
    createExecutor.mockResolvedValueOnce(createdExecutor);
    createExecutorProfile.mockResolvedValueOnce(createdProfile);

    await expect(createKubernetesExecutorPair(createInput())).resolves.toEqual({
      executor: createdExecutor,
      profile: createdProfile,
    });
    expect(kubernetesExecutorSettingsPath(createdExecutor.id)).toBe(
      "/settings/executors/k8s/executor%2Fprimary",
    );
  });

  it("deletes the just-created executor when profile creation fails", async () => {
    const profileFailure = new Error("profile failed");
    createExecutor.mockResolvedValueOnce({
      id: EXECUTOR_ID,
      name: "Cluster",
      type: "k8s",
      config: {},
    });
    createExecutorProfile.mockRejectedValueOnce(profileFailure);
    deleteExecutor.mockResolvedValueOnce({ success: true });

    await expect(createKubernetesExecutorPair(createInput())).rejects.toBe(profileFailure);
    expect(deleteExecutor).toHaveBeenCalledWith(EXECUTOR_ID);
  });

  it("reports both profile and rollback failures when compensation fails", async () => {
    const profileFailure = new Error("profile failed");
    const rollbackFailure = new Error("rollback failed");
    createExecutor.mockResolvedValueOnce({
      id: EXECUTOR_ID,
      name: "Cluster",
      type: "k8s",
      config: {},
    });
    createExecutorProfile.mockRejectedValueOnce(profileFailure);
    deleteExecutor.mockRejectedValueOnce(rollbackFailure);

    const failure = await createKubernetesExecutorPair(createInput()).catch((cause) => cause);

    expect(failure).toBeInstanceOf(AggregateError);
    expect((failure as AggregateError).errors).toEqual([profileFailure, rollbackFailure]);
  });

  it("treats a rejected rollback result as a compensation failure", async () => {
    const profileFailure = new Error("profile failed");
    createExecutor.mockResolvedValueOnce({
      id: EXECUTOR_ID,
      name: "Cluster",
      type: "k8s",
      config: {},
    });
    createExecutorProfile.mockRejectedValueOnce(profileFailure);
    deleteExecutor.mockResolvedValueOnce({ success: false });

    const failure = await createKubernetesExecutorPair(createInput()).catch((cause) => cause);

    expect(failure).toBeInstanceOf(AggregateError);
    expect((failure as AggregateError).errors[0]).toBe(profileFailure);
    expect((failure as AggregateError).errors[1]).toBeInstanceOf(
      KubernetesExecutorRollbackRejectedError,
    );
  });

  it("upserts a created executor without duplicating the store row", () => {
    expect(upsertExecutor([executor("one")], executor("two"))).toEqual([
      executor("one"),
      executor("two"),
    ]);
    expect(upsertExecutor([executor("one")], executor("one", "Updated"))).toEqual([
      executor("one", "Updated"),
    ]);
  });
});

describe("Kubernetes diagnostic and session hooks", () => {
  it("settles test loading and exposes the normalized diagnostic result", async () => {
    const response = { success: true, steps: [], warnings: [] };
    testKubernetesConnection.mockResolvedValueOnce(response);
    const { result } = renderHook(() => useKubernetesDiagnostics());

    let pending: Promise<unknown> | undefined;
    act(() => {
      pending = result.current.run({ config: { auth_mode: "in_cluster" } });
    });
    expect(result.current.testing).toBe(true);
    await act(async () => pending);

    expect(result.current.testing).toBe(false);
    expect(result.current.result).toEqual(response);
    expect(result.current.error).toBeNull();
  });

  it("clears stale results and loading state after a failed test", async () => {
    const failure = new Error("forbidden");
    testKubernetesConnection.mockRejectedValueOnce(failure);
    const { result } = renderHook(() => useKubernetesDiagnostics());

    await act(async () => {
      await expect(result.current.run({ config: {} })).rejects.toBe(failure);
    });

    expect(result.current.testing).toBe(false);
    expect(result.current.result).toBeNull();
    expect(result.current.error).toBe(failure);
  });

  it("returns authoritative session rows from a manual refresh", async () => {
    const rows = [{ session_id: "session-1", task_id: "task-1", restarts: 0 }];
    listKubernetesSessions.mockResolvedValue(rows);
    const { result } = renderHook(() => useKubernetesSessions(EXECUTOR_ID));

    await waitFor(() => expect(result.current.sessions).toEqual(rows));
    await expect(result.current.refresh()).resolves.toEqual(rows);
  });

  it("does not request Kubernetes sessions when the hook is disabled", async () => {
    const { result } = renderHook(() => useKubernetesSessions("docker-1", false));

    await expect(result.current.refresh()).resolves.toEqual([]);
    expect(listKubernetesSessions).not.toHaveBeenCalled();
  });

  it("loads the authoritative session impact without refreshing detailed rows", async () => {
    getKubernetesSessionImpact.mockResolvedValueOnce({ active_session_count: 4 });
    const { result } = renderHook(() => useKubernetesSessionImpact(EXECUTOR_ID));

    await expect(result.current.loadActiveSessionCount()).resolves.toBe(4);
    expect(getKubernetesSessionImpact).toHaveBeenCalledWith(EXECUTOR_ID, { cache: "no-store" });
    expect(listKubernetesSessions).not.toHaveBeenCalled();
  });
});

function createInput() {
  return {
    name: "Cluster",
    config: { auth_mode: "in_cluster" },
    profileName: "Default",
    profileConfig: { platform: "linux/amd64" },
  };
}

function profile(id: string, executorId: string) {
  return {
    id,
    executor_id: executorId,
    name: "Default",
    prepare_script: "",
    cleanup_script: "",
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}
