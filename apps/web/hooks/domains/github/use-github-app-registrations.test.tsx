import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { GitHubAppRegistrationCatalog } from "@/lib/types/github";
import {
  parseGitHubCallbackResult,
  useGitHubAppRegistrations,
} from "./use-github-app-registrations";

const apiMocks = vi.hoisted(() => ({
  fetch: vi.fn(),
  deleteRegistration: vi.fn(),
  importRegistration: vi.fn(),
  prepareImport: vi.fn(),
  renameRegistration: vi.fn(),
  startInstall: vi.fn(),
  startManifest: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchGitHubAppRegistrations: apiMocks.fetch,
  deleteGitHubAppRegistration: apiMocks.deleteRegistration,
  importGitHubAppRegistration: apiMocks.importRegistration,
  prepareGitHubAppImport: apiMocks.prepareImport,
  renameGitHubAppRegistration: apiMocks.renameRegistration,
  startGitHubAppInstall: apiMocks.startInstall,
  startGitHubAppManifest: apiMocks.startManifest,
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

function catalog(workspaceId: string): GitHubAppRegistrationCatalog {
  return { workspace_id: workspaceId, registrations: [] };
}

function wrapper({ children }: { children: React.ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

const workspaceA = "workspace-a";
const workspaceB = "workspace-b";

afterEach(() => {
  vi.clearAllMocks();
});

describe("useGitHubAppRegistrations workspace isolation", () => {
  it("never exposes a prior workspace response after switching workspaces", async () => {
    const first = deferred<GitHubAppRegistrationCatalog>();
    const second = deferred<GitHubAppRegistrationCatalog>();
    apiMocks.fetch.mockImplementation((workspaceId: string) =>
      workspaceId === workspaceA ? first.promise : second.promise,
    );

    const { result, rerender } = renderHook(
      ({ workspaceId }) => useGitHubAppRegistrations(workspaceId),
      { wrapper, initialProps: { workspaceId: workspaceA } },
    );
    await waitFor(() => expect(apiMocks.fetch).toHaveBeenCalledWith(workspaceA, expect.anything()));

    rerender({ workspaceId: workspaceB });
    expect(result.current.catalog).toBeNull();
    await waitFor(() => expect(apiMocks.fetch).toHaveBeenCalledWith(workspaceB, expect.anything()));

    await act(async () => second.resolve(catalog(workspaceB)));
    await waitFor(() => expect(result.current.catalog?.workspace_id).toBe(workspaceB));
    await act(async () => first.resolve(catalog(workspaceA)));
    expect(result.current.catalog?.workspace_id).toBe(workspaceB);
  });

  it("keeps the newest same-workspace refresh result", async () => {
    const first = deferred<GitHubAppRegistrationCatalog>();
    const second = deferred<GitHubAppRegistrationCatalog>();
    apiMocks.fetch.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const { result } = renderHook(() => useGitHubAppRegistrations(workspaceA), { wrapper });
    await waitFor(() => expect(apiMocks.fetch).toHaveBeenCalledTimes(1));
    let refreshing!: Promise<void>;
    act(() => {
      refreshing = result.current.refresh();
    });
    await waitFor(() => expect(apiMocks.fetch).toHaveBeenCalledTimes(2));

    await act(async () => second.resolve(catalog("workspace-a-new")));
    await refreshing;
    await act(async () => first.resolve(catalog("workspace-a-old")));
    expect(result.current.catalog?.workspace_id).toBe("workspace-a-new");
  });
});

describe("parseGitHubCallbackResult", () => {
  it("accepts the current workspace and rejects a foreign workspace callback", () => {
    expect(
      parseGitHubCallbackResult(
        new URLSearchParams(`github_result=app_registered&workspace_id=${workspaceA}`),
        workspaceA,
      ),
    ).toEqual({ code: "app_registered", workspace_id: workspaceA });
    expect(
      parseGitHubCallbackResult(
        new URLSearchParams(`github_result=app_registered&workspace_id=${workspaceB}`),
        workspaceA,
      ),
    ).toBeNull();
  });
});
