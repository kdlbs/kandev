import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ listUserOrgs: vi.fn() }));
vi.mock("@/lib/api/domains/github-api", () => ({ listUserOrgs: mocks.listUserOrgs }));

import { useGitHubOrgs } from "./use-github-orgs";

const org = (login: string) => ({ login, avatar_url: "" });

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

describe("useGitHubOrgs", () => {
  it("loads the workspace's organizations", async () => {
    mocks.listUserOrgs.mockResolvedValue({ orgs: [org("octocat"), org("kdlbs")] });
    const { result } = renderHook(() => useGitHubOrgs("ws-1"));
    expect(result.current.loading).toBe(true);
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mocks.listUserOrgs).toHaveBeenCalledWith("ws-1");
    expect(result.current.orgs.map((o) => o.login)).toEqual(["octocat", "kdlbs"]);
    expect(result.current.error).toBeNull();
  });

  it("does not request without a workspace", () => {
    const { result } = renderHook(() => useGitHubOrgs(undefined));
    expect(mocks.listUserOrgs).not.toHaveBeenCalled();
    expect(result.current.loading).toBe(false);
    expect(result.current.orgs).toEqual([]);
  });

  it("reports a load failure and recovers on refresh", async () => {
    mocks.listUserOrgs.mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useGitHubOrgs("ws-1"));
    await waitFor(() => expect(result.current.error?.message).toBe("boom"));
    expect(result.current.orgs).toEqual([]);

    mocks.listUserOrgs.mockResolvedValue({ orgs: [org("kdlbs")] });
    act(() => result.current.refresh());
    await waitFor(() => expect(result.current.orgs.map((o) => o.login)).toEqual(["kdlbs"]));
    expect(result.current.error).toBeNull();
    expect(mocks.listUserOrgs).toHaveBeenCalledTimes(2);
  });
});
