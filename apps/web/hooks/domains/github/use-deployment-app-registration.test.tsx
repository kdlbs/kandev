import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DeploymentGitHubAppStatus } from "@/lib/types/github";

const apiMocks = vi.hoisted(() => ({
  deleteRegistration: vi.fn(),
  fetchRegistration: vi.fn(),
  startRegistration: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  deleteDeploymentAppRegistration: apiMocks.deleteRegistration,
  fetchDeploymentAppRegistration: apiMocks.fetchRegistration,
  startDeploymentAppRegistration: apiMocks.startRegistration,
}));

import { useDeploymentAppRegistration } from "./use-deployment-app-registration";

const managedStatus: DeploymentGitHubAppStatus = {
  source: "managed",
  state: "ready",
  ready: true,
  read_only: false,
};

beforeEach(() => {
  apiMocks.deleteRegistration.mockReset();
  apiMocks.fetchRegistration.mockReset();
  apiMocks.startRegistration.mockReset();
});

afterEach(() => cleanup());

describe("useDeploymentAppRegistration", () => {
  it("clears deleted registration state when the follow-up refresh fails", async () => {
    apiMocks.fetchRegistration
      .mockResolvedValueOnce(managedStatus)
      .mockRejectedValueOnce(new Error("refresh unavailable"));
    apiMocks.deleteRegistration.mockResolvedValue({ deleted: true });
    const { result } = renderHook(() => useDeploymentAppRegistration());
    await waitFor(() => expect(result.current.status).toEqual(managedStatus));

    let removal: Awaited<ReturnType<typeof result.current.remove>> | undefined;
    await act(async () => {
      removal = await result.current.remove();
    });

    expect(removal).toEqual({ deleted: true, refreshed: false });
    expect(result.current.status).toBeNull();
    expect(result.current.error).toBe("refresh unavailable");
    expect(result.current.mutating).toBe(false);
  });

  it("does not retain a prior registration after any failed reload", async () => {
    apiMocks.fetchRegistration
      .mockResolvedValueOnce(managedStatus)
      .mockRejectedValueOnce(new Error("status unavailable"));
    const { result } = renderHook(() => useDeploymentAppRegistration());
    await waitFor(() => expect(result.current.status).toEqual(managedStatus));

    await act(async () => {
      await result.current.reload();
    });

    expect(result.current.status).toBeNull();
    expect(result.current.error).toBe("status unavailable");
  });
});
