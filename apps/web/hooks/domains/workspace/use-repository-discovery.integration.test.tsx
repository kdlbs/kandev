import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import {
  getRepositoryDiscoveryAction,
  refreshRepositoryDiscoveryAction,
} from "@/app/actions/workspaces";
import { useDiscoveredRepositories } from "@/app/office/projects/use-discovered-repositories";
import type { RepositoryDiscoveryResponse } from "@/lib/types/http";
import { repositoryDiscoveryCoordinator, useRepositoryDiscovery } from "./use-repository-discovery";

vi.mock("@/app/actions/workspaces", () => ({
  getRepositoryDiscoveryAction: vi.fn(),
  refreshRepositoryDiscoveryAction: vi.fn(),
}));

function deferred() {
  let resolve!: (value: RepositoryDiscoveryResponse) => void;
  const promise = new Promise<RepositoryDiscoveryResponse>((accept) => {
    resolve = accept;
  });
  return { promise, resolve };
}

function response(root: string): RepositoryDiscoveryResponse {
  return {
    roots: [root],
    repositories: [{ path: `${root}/repo`, name: root }],
    total: 1,
    root_states: [],
    scan_time: new Date().toISOString(),
    refreshing: false,
    cached: false,
    home_confirmation_required: false,
    failed_roots: [],
  };
}

describe("shared repository discovery and Office choices", () => {
  beforeEach(() => {
    repositoryDiscoveryCoordinator.dispose();
    vi.mocked(getRepositoryDiscoveryAction).mockReset();
    vi.mocked(refreshRepositoryDiscoveryAction).mockReset();
  });

  afterEach(() => repositoryDiscoveryCoordinator.dispose());

  // @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.13
  it("keeps current shared choices and flags after an obsolete mount read", async () => {
    const cached = deferred();
    const scan = deferred();
    vi.mocked(getRepositoryDiscoveryAction).mockReturnValue(cached.promise);
    vi.mocked(refreshRepositoryDiscoveryAction).mockReturnValue(scan.promise);
    const { result, unmount } = renderHook(() => ({
      discovery: useRepositoryDiscovery("workspace"),
      office: useDiscoveredRepositories(true, "workspace"),
    }));
    let newer!: ReturnType<typeof result.current.discovery.refresh>;
    const older = repositoryDiscoveryCoordinator.load("workspace");
    act(() => {
      newer = result.current.discovery.refresh();
    });
    expect(result.current.discovery.isLoading).toBe(true);
    expect(result.current.discovery.isRefreshing).toBe(true);
    const current = response("/current");
    await act(async () => {
      scan.resolve(current);
      await newer;
    });
    expect(result.current.office).toEqual(current.repositories);
    await act(async () => {
      cached.resolve({ ...response("/obsolete"), cached: true });
      await older;
    });
    const exposed = result.current;
    unmount();
    expect(exposed.office).toEqual(current.repositories);
    expect(exposed.discovery).toMatchObject({
      repositories: current.repositories,
      roots: current.roots,
      scanTime: current.scan_time,
      cached: false,
      hasSnapshot: true,
      isLoading: false,
      isRefreshing: false,
      rootStates: [],
      failedRoots: [],
      error: null,
    });
    expect(getRepositoryDiscoveryAction).toHaveBeenCalledTimes(1);
    expect(refreshRepositoryDiscoveryAction).toHaveBeenCalledTimes(1);
  });
});
