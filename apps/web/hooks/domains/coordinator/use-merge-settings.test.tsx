/* eslint-disable max-lines-per-function -- one describe covers the hook's read, draft and save paths */
import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { CoordinatorSettings } from "@/lib/api/domains/coordinator-api";
import type { MergeRepo } from "@/lib/api/domains/coordinator-merge-api";

const getMock = vi.fn();
const putMock = vi.fn();
const refreshMock = vi.fn();
const toastError = vi.fn();
let contributor: { isDirty: boolean; save: () => Promise<void>; discard: () => void } | null = null;

vi.mock("@/lib/api/domains/coordinator-api", () => ({
  getCoordinatorSettings: (...args: unknown[]) => getMock(...args),
  putCoordinatorSettings: (...args: unknown[]) => putMock(...args),
}));
vi.mock("@/lib/api/domains/coordinator-merge-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/coordinator-merge-api")>()),
  refreshMergeProtection: (...args: unknown[]) => refreshMock(...args),
}));
vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: (c: typeof contributor) => {
    contributor = c;
  },
}));
vi.mock("@/lib/toast/sonner", () => ({ toast: { error: (...a: unknown[]) => toastError(...a) } }));
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

import { useMergeSettings } from "./use-merge-settings";

function repo(id: string, policy: MergeRepo["policy"], active = true): MergeRepo {
  return { repository_id: id, name: id, policy, automatic_active: active };
}

function settings(repos: MergeRepo[], revision = 1): CoordinatorSettings {
  return {
    policy: { actions: {} },
    policy_revision: revision,
    watches: { scope: "all" },
    merge: { repositories: repos, protection: [] },
  } as unknown as CoordinatorSettings;
}

function mount(enabled = true) {
  return renderHook(() =>
    useMergeSettings({ workspaceId: "w1", coordinatorId: "c1", canManage: true, enabled }),
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  contributor = null;
});

describe("useMergeSettings", () => {
  it("reads the repositories and reports ready", async () => {
    getMock.mockResolvedValue(settings([repo("r1", "denied")]));
    const { result } = mount();
    expect(result.current.status).toBe("loading");
    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(result.current.repos).toHaveLength(1);
  });

  it("is off when the settings carry no merge member", async () => {
    getMock.mockResolvedValue({ ...settings([]), merge: undefined });
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("off"));
  });

  it("shows a load failure and recovers on retry", async () => {
    getMock.mockRejectedValueOnce(new Error("down"));
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("error"));
    getMock.mockResolvedValue(settings([repo("r1", "denied")]));
    act(() => result.current.retry());
    await waitFor(() => expect(result.current.status).toBe("ready"));
  });

  it("saves only the changed repositories with the read revision", async () => {
    const stored = [repo("r1", "denied"), repo("r2", "requires_approval")];
    getMock.mockResolvedValue(settings(stored, 7));
    putMock.mockResolvedValue(settings([repo("r1", "automatic"), stored[1]], 8));
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.choosePolicy(stored[0], "automatic"));
    expect(contributor?.isDirty).toBe(true);
    await act(async () => {
      await contributor?.save();
    });
    expect(putMock).toHaveBeenCalledWith("w1", "c1", {
      policy_revision: 7,
      merge: { repositories: [{ repository_id: "r1", policy: "automatic" }] },
    });
    expect(contributor?.isDirty).toBe(false);
  });

  it("sends replace_principal for an inactive automatic repository", async () => {
    const inactive = repo("r1", "automatic", false);
    getMock.mockResolvedValue(settings([inactive]));
    putMock.mockResolvedValue(settings([repo("r1", "automatic", true)], 2));
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.replacePrincipal(inactive));
    await act(async () => {
      await contributor?.save();
    });
    expect(putMock.mock.calls[0][2].merge.repositories).toEqual([
      { repository_id: "r1", policy: "automatic", replace_principal: true },
    ]);
  });

  it("surfaces principal_valid without a generic toast", async () => {
    const inactive = repo("r1", "automatic", false);
    getMock.mockResolvedValue(settings([inactive]));
    putMock.mockRejectedValue(new ApiError("x", 409, { code: "principal_valid" }));
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.replacePrincipal(inactive));
    await act(async () => {
      await contributor?.save().catch(() => undefined);
    });
    expect(result.current.saveError).toBe("principal_valid");
    expect(toastError).not.toHaveBeenCalled();
  });

  it("retries once when only the revision moved", async () => {
    const stored = [repo("r1", "denied")];
    getMock.mockResolvedValueOnce(settings(stored, 1)).mockResolvedValueOnce(settings(stored, 2));
    putMock
      .mockRejectedValueOnce(new ApiError("x", 409, { code: "revision_conflict" }))
      .mockResolvedValueOnce(settings([repo("r1", "automatic")], 3));
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.choosePolicy(stored[0], "automatic"));
    await act(async () => {
      await contributor?.save();
    });
    expect(putMock).toHaveBeenCalledTimes(2);
    expect(putMock.mock.calls[1][2].policy_revision).toBe(2);
  });

  it("reports a conflict and adopts the fresh read when the policy changed elsewhere", async () => {
    const stored = [repo("r1", "denied")];
    getMock
      .mockResolvedValueOnce(settings(stored, 1))
      .mockResolvedValueOnce(settings([repo("r1", "requires_approval")], 2));
    putMock.mockRejectedValue(new ApiError("x", 409, { code: "revision_conflict" }));
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.choosePolicy(stored[0], "automatic"));
    await act(async () => {
      await contributor?.save().catch(() => undefined);
    });
    expect(putMock).toHaveBeenCalledTimes(1);
    expect(result.current.saveError).toBe("conflict");
    expect(result.current.repos[0].policy).toBe("requires_approval");
  });

  it("toasts an unclassified failure", async () => {
    const stored = [repo("r1", "denied")];
    getMock.mockResolvedValue(settings(stored));
    putMock.mockRejectedValue(new ApiError("x", 500, null));
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.choosePolicy(stored[0], "automatic"));
    await act(async () => {
      await contributor?.save().catch(() => undefined);
    });
    expect(result.current.saveError).toBe("failed");
    expect(toastError).toHaveBeenCalledWith("coordinator:mergeSaveFailed");
  });

  it("refreshes protection and reports a failed refresh", async () => {
    getMock.mockResolvedValue(settings([repo("r1", "denied")]));
    const observation = {
      repository_id: "r1",
      branch: "main",
      pipeline: "enforced",
      approvals: "unknown",
      threads: "enforced",
      observed_at: "2026-01-01T00:00:00Z",
    };
    refreshMock.mockResolvedValueOnce({ protection: [observation] });
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    await act(async () => {
      await result.current.refreshProtection();
    });
    expect(result.current.protection).toEqual([observation]);
    refreshMock.mockRejectedValueOnce(new Error("provider"));
    await act(async () => {
      await result.current.refreshProtection();
    });
    expect(result.current.refreshFailed).toBe(true);
    expect(result.current.protection).toEqual([observation]);
  });
});
