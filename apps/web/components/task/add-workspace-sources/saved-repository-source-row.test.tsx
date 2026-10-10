import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, renderHook, screen } from "@testing-library/react";
import {
  addDesktopDiscoveryRootAction,
  getRepositoryDiscoveryAction,
} from "@/app/actions/workspaces";
import type { RepositoryDiscoveryResponse } from "@/lib/types/http";
import {
  repositoryDiscoveryCoordinator,
  useRepositoryDiscovery,
} from "@/hooks/domains/workspace/use-repository-discovery";
import { SavedRepositorySourceRow } from "./saved-repository-source-row";

const action = vi.hoisted(() => ({ pending: null as Promise<void> | null, toast: vi.fn() }));

vi.mock("@/app/actions/workspaces", () => ({
  addDesktopDiscoveryRootAction: vi.fn(),
  getRepositoryDiscoveryAction: vi.fn(),
  refreshRepositoryDiscoveryAction: vi.fn(),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { upsertRepository: () => void }) => unknown) =>
    selector({ upsertRepository: vi.fn() }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: action.toast }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/components/create-local-repository-surface", () => ({
  CreateLocalRepositorySurface: () => null,
}));
vi.mock("@/components/repository-discovery-dialog", () => ({
  RepositoryDiscoveryDialog: () => null,
}));
vi.mock("@/components/task-create-dialog-workspace-repo-chips", () => ({
  WorkspaceRepoChips: ({
    onAddHomeAndOpenDiscovery,
  }: {
    onAddHomeAndOpenDiscovery: () => Promise<void>;
  }) => (
    <button
      onClick={() => {
        action.pending = onAddHomeAndOpenDiscovery();
      }}
    >
      Add Home
    </button>
  ),
}));

function snapshot(root: string): RepositoryDiscoveryResponse {
  return {
    roots: [root],
    repositories: [{ path: `${root}/repo`, name: root }],
    total: 1,
    root_states: [],
    scan_time: new Date().toISOString(),
    desktop_runtime: true,
    refreshing: false,
    cached: false,
    failed_roots: [],
    home_confirmation_required: false,
  };
}

beforeEach(() => {
  repositoryDiscoveryCoordinator.dispose();
  vi.resetAllMocks();
  action.pending = null;
});
afterEach(() => {
  cleanup();
  repositoryDiscoveryCoordinator.dispose();
});

// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.14
it.each([true, false])(
  "saved-source Add Home keeps real shared state correct with success=%s",
  async (success) => {
    let resolveOld!: (value: RepositoryDiscoveryResponse) => void;
    const old = new Promise<RepositoryDiscoveryResponse>((resolve) => {
      resolveOld = resolve;
    });
    const baseline = snapshot("/baseline");
    const current = snapshot("/home");
    vi.mocked(getRepositoryDiscoveryAction).mockResolvedValue(baseline);
    if (success) {
      vi.mocked(addDesktopDiscoveryRootAction).mockResolvedValue({
        id: "home",
        path: "/home",
        display_path: "~",
        state: "connected",
      });
    } else vi.mocked(addDesktopDiscoveryRootAction).mockRejectedValue(new Error("mutation failed"));
    render(
      <SavedRepositorySourceRow
        row={{ key: "row", kind: "repository" }}
        repositories={[]}
        discoveredRepositories={[]}
        workspaceId="A"
        canCreateRepository={false}
        repositoriesRefreshing={false}
        onRefreshRepositories={vi.fn()}
        onUpdate={vi.fn()}
      />,
    );
    const observer = renderHook(() => useRepositoryDiscovery("A"));
    await act(async () => {
      await repositoryDiscoveryCoordinator.load("A");
    });
    vi.mocked(getRepositoryDiscoveryAction).mockReturnValueOnce(old).mockResolvedValue(current);
    let oldCall!: Promise<RepositoryDiscoveryResponse | null>;
    act(() => {
      oldCall = repositoryDiscoveryCoordinator.load("A");
    });
    try {
      await act(async () => {
        fireEvent.click(screen.getByRole("button", { name: "Add Home" }));
        await Promise.resolve();
      });
      expect(addDesktopDiscoveryRootAction).toHaveBeenCalledWith("~");
      // Only successful mutation starts another transport while the old one is pending.
      expect(getRepositoryDiscoveryAction).toHaveBeenCalledTimes(success ? 3 : 2);
      await act(async () => {
        if (success) await action.pending;
        resolveOld(snapshot("/old"));
        await Promise.all([oldCall, action.pending]);
      });
      expect(observer.result.current.roots).toEqual(success ? ["/home"] : ["/old"]);
      expect(observer.result.current.isLoading).toBe(false);
      expect(action.toast).toHaveBeenCalledTimes(success ? 0 : 1);
    } finally {
      await act(async () => {
        resolveOld(baseline);
        await Promise.all([oldCall, action.pending]);
      });
      observer.unmount();
    }
  },
);
