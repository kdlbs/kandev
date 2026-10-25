import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SettingsSaveContributor } from "../settings-save-provider";

const { discard, loadActiveSessionCount, refreshDetailedSessions, save, registerContributor } =
  vi.hoisted(() => ({
    discard: vi.fn(),
    loadActiveSessionCount: vi.fn(),
    refreshDetailedSessions: vi.fn(),
    save: vi.fn(),
    registerContributor: vi.fn(),
  }));

vi.mock("@/hooks/domains/settings/use-kubernetes-settings", () => ({
  useKubernetesSessionImpact: () => ({ loadActiveSessionCount }),
  useKubernetesSessions: () => ({ refresh: refreshDetailedSessions }),
}));

vi.mock("../settings-save-provider", async (importOriginal) => {
  const original = await importOriginal<typeof import("../settings-save-provider")>();
  return {
    ...original,
    useSettingsSaveContributor: (contributor: SettingsSaveContributor) =>
      registerContributor(contributor),
  };
});

import { useExecutorProfileSaveContributor } from "./use-executor-profile-save-contributor";

beforeEach(() => {
  discard.mockReset();
  loadActiveSessionCount.mockReset().mockResolvedValue(2);
  refreshDetailedSessions.mockReset().mockResolvedValue([]);
  save.mockReset().mockResolvedValue(undefined);
  registerContributor.mockReset();
  vi.stubGlobal(
    "confirm",
    vi.fn(() => true),
  );
});

describe("Kubernetes executor profile save contribution", () => {
  it("confirms against authoritative executor impact without refreshing detailed rows", async () => {
    renderHook(() =>
      useExecutorProfileSaveContributor({
        executorId: "executor-1",
        profileId: "profile-1",
        payload: {
          name: "Default",
          prepare_script: "",
          cleanup_script: "",
          env_vars: [],
        },
        isRemote: false,
        gitIdentityLoaded: true,
        isKubernetes: true,
        canManageKubernetes: true,
        save,
        discard,
      }),
    );
    const contributor = registerContributor.mock.calls.at(-1)?.[0] as
      | SettingsSaveContributor
      | undefined;
    expect(contributor).toBeDefined();

    await act(async () => contributor?.save(contributor.revision));

    expect(loadActiveSessionCount).toHaveBeenCalledOnce();
    expect(refreshDetailedSessions).not.toHaveBeenCalled();
    expect(save).toHaveBeenCalledOnce();
    expect(contributor?.discard).toBe(discard);
  });
});
