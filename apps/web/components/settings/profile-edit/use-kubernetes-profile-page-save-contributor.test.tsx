import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { SettingsSaveContributor } from "../settings-save-provider";
import { createDefaultKubernetesExecutorForm } from "../kubernetes-config";

const mocks = vi.hoisted(() => ({
  loadActiveSessionCount: vi.fn(),
  registerContributor: vi.fn(),
}));

vi.mock("@/hooks/domains/settings/use-kubernetes-settings", () => ({
  useKubernetesSessionImpact: () => ({
    loadActiveSessionCount: mocks.loadActiveSessionCount,
  }),
}));

vi.mock("../settings-save-provider", async (importOriginal) => {
  const original = await importOriginal<typeof import("../settings-save-provider")>();
  return {
    ...original,
    useSettingsSaveContributor: (contributor: SettingsSaveContributor) =>
      mocks.registerContributor(contributor),
  };
});

import { useKubernetesProfilePageSaveContributor } from "./use-kubernetes-profile-page-save-contributor";

const initialConnection = createDefaultKubernetesExecutorForm();
const EXECUTOR_ID = "executor-1";
const initialProfile = {
  name: "Default",
  prepare_script: "",
  cleanup_script: "",
  env_vars: [],
};

beforeEach(() => {
  mocks.loadActiveSessionCount.mockReset().mockResolvedValue(2);
  mocks.registerContributor.mockReset();
  vi.stubGlobal(
    "confirm",
    vi.fn(() => true),
  );
});

describe("Kubernetes profile page save contribution", () => {
  it("confirms once, saves connection before profile, and records each successful baseline", async () => {
    const events: string[] = [];
    const markConnectionSaved = vi.fn(() => events.push("connection-baseline"));
    const saveConnection = vi.fn(async () => {
      events.push("connection-save");
    });
    const saveProfile = vi.fn(async () => {
      events.push("profile-save");
    });
    const { rerender } = renderHook(
      ({ connection, profile }) =>
        useKubernetesProfilePageSaveContributor({
          enabled: true,
          executorId: EXECUTOR_ID,
          profileId: "profile-1",
          connectionForm: connection,
          connectionBaseline: initialConnection,
          profilePayload: profile,
          isRemote: false,
          gitIdentityLoaded: true,
          connectionLoaded: true,
          canManage: true,
          saveConnection,
          markConnectionSaved,
          saveProfile,
          discard: vi.fn(),
        }),
      { initialProps: { connection: initialConnection, profile: initialProfile } },
    );
    rerender({
      connection: { ...initialConnection, namespace: "production" },
      profile: { ...initialProfile, prepare_script: "echo ready" },
    });
    const contributor = mocks.registerContributor.mock.calls.at(-1)?.[0] as SettingsSaveContributor;

    await act(async () => contributor.save(contributor.revision));

    expect(mocks.loadActiveSessionCount).toHaveBeenCalledOnce();
    expect(window.confirm).toHaveBeenCalledOnce();
    expect(events).toEqual(["connection-save", "connection-baseline", "profile-save"]);
  });

  it("marks the connection saved before surfacing a later profile failure", async () => {
    const markConnectionSaved = vi.fn();
    const profileFailure = new Error("profile failed");
    const { rerender } = renderHook(
      ({ connection, profile }) =>
        useKubernetesProfilePageSaveContributor({
          enabled: true,
          executorId: EXECUTOR_ID,
          profileId: "profile-1",
          connectionForm: connection,
          connectionBaseline: initialConnection,
          profilePayload: profile,
          isRemote: false,
          gitIdentityLoaded: true,
          connectionLoaded: true,
          canManage: true,
          saveConnection: vi.fn().mockResolvedValue(undefined),
          markConnectionSaved,
          saveProfile: vi.fn().mockRejectedValue(profileFailure),
          discard: vi.fn(),
        }),
      { initialProps: { connection: initialConnection, profile: initialProfile } },
    );
    const changedConnection = { ...initialConnection, namespace: "production" };
    rerender({
      connection: changedConnection,
      profile: { ...initialProfile, prepare_script: "echo ready" },
    });
    const contributor = mocks.registerContributor.mock.calls.at(-1)?.[0] as SettingsSaveContributor;

    await expect(act(async () => contributor.save(contributor.revision))).rejects.toThrow(
      profileFailure,
    );

    expect(markConnectionSaved).toHaveBeenCalledWith(changedConnection);
  });
});

describe("Kubernetes profile page save timing", () => {
  it("waits for the authoritative executor connection before reporting ready", () => {
    const { result, rerender } = renderHook(
      ({ connectionLoaded }) =>
        useKubernetesProfilePageSaveContributor({
          enabled: true,
          executorId: EXECUTOR_ID,
          profileId: "profile-1",
          connectionForm: initialConnection,
          connectionBaseline: initialConnection,
          profilePayload: initialProfile,
          isRemote: false,
          gitIdentityLoaded: true,
          connectionLoaded,
          canManage: true,
          saveConnection: vi.fn(),
          markConnectionSaved: vi.fn(),
          saveProfile: vi.fn(),
          discard: vi.fn(),
        }),
      { initialProps: { connectionLoaded: false } },
    );

    expect(result.current).toBe(false);
    rerender({ connectionLoaded: true });
    expect(result.current).toBe(true);
  });

  it("saves the newest workload payload when it changes during the connection save", async () => {
    let releaseConnection: (() => void) | undefined;
    const saveConnection = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          releaseConnection = resolve;
        }),
    );
    const saveProfile = vi.fn().mockResolvedValue(undefined);
    const { rerender } = renderHook(
      ({ connection, profile }) =>
        useKubernetesProfilePageSaveContributor({
          enabled: true,
          executorId: EXECUTOR_ID,
          profileId: "profile-1",
          connectionForm: connection,
          connectionBaseline: initialConnection,
          profilePayload: profile,
          isRemote: false,
          gitIdentityLoaded: true,
          connectionLoaded: true,
          canManage: true,
          saveConnection,
          markConnectionSaved: vi.fn(),
          saveProfile,
          discard: vi.fn(),
        }),
      {
        initialProps: {
          connection: initialConnection,
          profile: initialProfile,
        },
      },
    );
    rerender({
      connection: { ...initialConnection, namespace: "production" },
      profile: { ...initialProfile, prepare_script: "echo first" },
    });
    const contributor = mocks.registerContributor.mock.calls.at(-1)?.[0] as SettingsSaveContributor;
    let pending: Promise<void> | undefined;
    act(() => {
      pending = Promise.resolve(contributor.save(contributor.revision));
    });
    await waitFor(() => expect(saveConnection).toHaveBeenCalledOnce());
    rerender({
      connection: { ...initialConnection, namespace: "production" },
      profile: { ...initialProfile, prepare_script: "echo newest" },
    });

    act(() => releaseConnection?.());
    await act(async () => pending);

    expect(saveProfile).toHaveBeenCalledWith({ ...initialProfile, prepare_script: "echo newest" });
  });
});
