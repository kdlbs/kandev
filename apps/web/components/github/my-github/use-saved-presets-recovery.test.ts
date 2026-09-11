import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { fetchGitHubWorkspaceSettings } from "@/lib/api/domains/github-api";
import { __resetSnapshotForTests, useSavedPresets } from "./use-saved-presets";

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: vi.fn(),
  updateUserSettings: vi.fn(),
}));
vi.mock("@/lib/api/domains/github-api", () => ({
  fetchGitHubWorkspaceSettings: vi.fn(),
  updateGitHubWorkspaceSettings: vi.fn(),
}));

const saved = {
  id: "saved-review",
  kind: "pr",
  label: "Reviews",
  customQuery: "review-requested:@me",
  repoFilter: "",
  isDefault: false,
  createdAt: "2026-09-11T00:00:00Z",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  vi.resetAllMocks();
  __resetSnapshotForTests();
});

// @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.1
describe.each([null, "workspace-1"])("saved-query recovery in %s", (workspaceId) => {
  const response = (presets: unknown[]) =>
    workspaceId
      ? { workspace_id: workspaceId, saved_presets: presets }
      : { settings: { github_saved_presets: presets } };
  const fetchSettings = () =>
    vi.mocked(workspaceId ? fetchGitHubWorkspaceSettings : fetchUserSettings);

  it("distinguishes loading, failure and a successful retry", async () => {
    const request = deferred<never>();
    fetchSettings()
      .mockReturnValueOnce(request.promise)
      .mockResolvedValueOnce(response([saved]) as never);
    const { result } = renderHook(() => useSavedPresets(workspaceId));
    expect(result.current.loading).toBe(true);
    await act(async () => request.reject(new Error("unavailable")));
    expect(result.current.error).toBe(true);
    expect(result.current.loading).toBe(false);
    await act(async () => result.current.retry());
    await waitFor(() => expect(result.current.presets).toEqual([saved]));
    expect(result.current.error).toBe(false);
    expect(result.current.loading).toBe(false);
    expect(fetchSettings()).toHaveBeenCalledTimes(2);
  });

  it("distinguishes a successful empty collection from unavailable settings", async () => {
    fetchSettings().mockResolvedValue(response([]) as never);
    const { result } = renderHook(() => useSavedPresets(workspaceId));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBe(false);
    expect(result.current.presets).toEqual([]);
  });
});

it("ignores a failed load from the previous workspace", async () => {
  const first = deferred<Awaited<ReturnType<typeof fetchGitHubWorkspaceSettings>>>();
  vi.mocked(fetchGitHubWorkspaceSettings)
    .mockReturnValueOnce(first.promise)
    .mockResolvedValueOnce({ saved_presets: [saved] } as never);
  const { result, rerender } = renderHook((id) => useSavedPresets(id), { initialProps: "first" });
  rerender("second");
  await waitFor(() => expect(result.current.presets).toEqual([saved]));
  await act(async () => first.reject(new Error("old workspace failed")));
  expect(result.current.error).toBe(false);
  expect(result.current.loading).toBe(false);
  expect(result.current.presets).toEqual([saved]);
});
