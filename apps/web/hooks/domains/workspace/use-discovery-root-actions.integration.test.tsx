import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { TFunction } from "i18next";
import {
  getRepositoryDiscoveryAction,
  refreshRepositoryDiscoveryAction,
  removeDesktopDiscoveryRootAction,
  addDesktopDiscoveryRootAction,
  confirmHomeDesktopDiscoveryAction,
  reconnectDesktopDiscoveryRootAction,
} from "@/app/actions/workspaces";
import { useDiscoveredRepositories } from "@/app/office/projects/use-discovered-repositories";
import type { RepositoryDiscoveryResponse } from "@/lib/types/http";
import { repositoryDiscoveryCoordinator, useRepositoryDiscovery } from "./use-repository-discovery";
import { useDiscoveryRootActions } from "./use-discovery-root-actions";

vi.mock("@/app/actions/workspaces", () => ({
  getRepositoryDiscoveryAction: vi.fn(),
  refreshRepositoryDiscoveryAction: vi.fn(),
  removeDesktopDiscoveryRootAction: vi.fn(),
  addDesktopDiscoveryRootAction: vi.fn(),
  confirmHomeDesktopDiscoveryAction: vi.fn(),
  reconnectDesktopDiscoveryRootAction: vi.fn(),
}));

function snapshot(root: string | null): RepositoryDiscoveryResponse {
  return {
    roots: root ? [root] : [],
    repositories: root ? [{ path: `${root}/repo`, name: root }] : [],
    total: root ? 1 : 0,
    root_states: root ? [{ id: "root-1", path: root, display_path: root, state: "connected" }] : [],
    scan_time: new Date().toISOString(),
    desktop_runtime: true,
    refreshing: false,
    cached: false,
    home_confirmation_required: false,
    failed_roots: [],
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((accept, fail) => {
    resolve = accept;
    reject = fail;
  });
  return { promise, resolve, reject };
}

const translate = ((key: string) => key) as TFunction;

beforeEach(() => {
  repositoryDiscoveryCoordinator.dispose();
  vi.resetAllMocks();
  const root = {
    id: "root",
    path: "/current",
    display_path: "/current",
    state: "connected" as const,
  };
  vi.mocked(addDesktopDiscoveryRootAction).mockResolvedValue(root);
  vi.mocked(confirmHomeDesktopDiscoveryAction).mockResolvedValue(root);
  vi.mocked(reconnectDesktopDiscoveryRootAction).mockResolvedValue(root);
  vi.mocked(removeDesktopDiscoveryRootAction).mockResolvedValue(undefined);
});
afterEach(() => repositoryDiscoveryCoordinator.dispose());

// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.14
it("does not restore removed roots or Office choices from a pre-mutation refresh", async () => {
  const older = deferred<RepositoryDiscoveryResponse>();
  vi.mocked(getRepositoryDiscoveryAction).mockResolvedValue(snapshot("/removed"));
  vi.mocked(refreshRepositoryDiscoveryAction)
    .mockReturnValueOnce(older.promise)
    .mockResolvedValue(snapshot(null));
  vi.mocked(removeDesktopDiscoveryRootAction).mockResolvedValue(undefined);
  const { result, unmount } = renderHook(() => {
    const discovery = useRepositoryDiscovery("workspace");
    return {
      discovery,
      actions: useDiscoveryRootActions(discovery, vi.fn(), translate),
      office: useDiscoveredRepositories(true, "workspace"),
    };
  });
  try {
    await act(async () => {
      await repositoryDiscoveryCoordinator.load("workspace");
    });
    expect(result.current.discovery.roots).toEqual(["/removed"]);
    let oldRead!: Promise<RepositoryDiscoveryResponse | null>;
    act(() => {
      oldRead = repositoryDiscoveryCoordinator.refresh("workspace", "stale_refresh");
    });
    let mutation!: Promise<void>;
    await act(async () => {
      mutation = result.current.actions.handleRemoveDiscoveryRoot("/removed");
      await Promise.resolve();
    });
    await act(async () => {
      older.resolve(snapshot("/removed"));
      await Promise.all([oldRead, mutation]);
    });
    expect(result.current.discovery.roots).toEqual([]);
    expect(result.current.office).toEqual([]);
    expect(refreshRepositoryDiscoveryAction).toHaveBeenCalledTimes(2);
    expect(result.current.discovery).toMatchObject({
      repositories: [],
      rootStates: [],
      failedRoots: [],
      hasSnapshot: true,
      isLoading: false,
      isRefreshing: false,
      error: null,
    });
  } finally {
    unmount();
  }
});

const actionNames = ["Add", "Home", "Reconnect", "Remove"] as const;
type ActionName = (typeof actionNames)[number];
type Actions = ReturnType<typeof useDiscoveryRootActions>;

function perform(actions: Actions, name: ActionName) {
  if (name === "Add") return actions.handleChooseDiscoveryRoot("/current");
  if (name === "Home") return actions.handleConfirmHomeDiscovery();
  if (name === "Reconnect") return actions.handleReconnectDiscoveryRoot("/baseline", "/current");
  return actions.handleRemoveDiscoveryRoot("/baseline");
}

function actionTransport(name: ActionName) {
  if (name === "Add") return vi.mocked(addDesktopDiscoveryRootAction);
  if (name === "Home") return vi.mocked(confirmHomeDesktopDiscoveryAction);
  if (name === "Reconnect") return vi.mocked(reconnectDesktopDiscoveryRootAction);
  return vi.mocked(removeDesktopDiscoveryRootAction);
}

async function mountConsumers() {
  vi.mocked(getRepositoryDiscoveryAction).mockResolvedValue(snapshot("/baseline"));
  vi.mocked(refreshRepositoryDiscoveryAction).mockResolvedValue(snapshot("/baseline"));
  const toast = vi.fn();
  const hook = renderHook(() => {
    const discovery = useRepositoryDiscovery("workspace");
    return {
      discovery,
      office: useDiscoveredRepositories(true, "workspace"),
      actions: useDiscoveryRootActions(discovery, toast, translate),
    };
  });
  await act(async () => {
    await repositoryDiscoveryCoordinator.load("workspace");
  });
  return { ...hook, toast };
}

// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.14
it.each(actionNames)(
  "%s synchronizes fresh shared state before its old same-kind read settles",
  async (name) => {
    const { result, unmount, toast } = await mountConsumers();
    const old = deferred<RepositoryDiscoveryResponse>();
    const fresh = deferred<RepositoryDiscoveryResponse>();
    const kind = name === "Remove" ? "refresh" : "load";
    const transport =
      name === "Remove" ? refreshRepositoryDiscoveryAction : getRepositoryDiscoveryAction;
    vi.mocked(transport).mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
    let oldCall!: Promise<RepositoryDiscoveryResponse | null>;
    let mutation!: Promise<void>;
    act(() => {
      oldCall = repositoryDiscoveryCoordinator[kind]("workspace");
    });
    await act(async () => {
      mutation = perform(result.current.actions, name);
      await Promise.resolve();
    });
    const current = snapshot(name === "Remove" ? null : "/current");
    try {
      expect(result.current.actions.isMutating).toBe(true);
      expect(result.current.actions.isConfirmingHomeDiscovery).toBe(name === "Home");
      expect(transport).toHaveBeenCalledTimes(name === "Remove" ? 2 : 3);
      await act(async () => {
        fresh.resolve(current);
        await mutation;
      });
      expect(result.current.office).toEqual(current.repositories);
      expect(result.current.discovery).toMatchObject({
        roots: current.roots,
        repositories: current.repositories,
        rootStates: current.root_states,
        scanTime: current.scan_time,
        cached: false,
        failedRoots: [],
        isLoading: false,
        isRefreshing: false,
        error: null,
        homeConfirmationRequired: false,
      });
      expect(result.current.actions).toMatchObject({
        isMutating: false,
        isConfirmingHomeDiscovery: false,
      });
      await act(async () => {
        old.reject(new Error("obsolete read failure"));
        await oldCall;
      });
      expect(result.current.office).toEqual(current.repositories);
      expect(result.current.discovery.error).toBeNull();
      expect(toast).not.toHaveBeenCalled();
    } finally {
      await act(async () => {
        old.resolve(snapshot("/cleanup"));
        fresh.resolve(current);
        await Promise.all([oldCall, mutation]);
      });
      unmount();
    }
  },
);

it.each(actionNames)(
  "failed %s keeps the pending read valid and resets mutation admission",
  async (name) => {
    const { result, unmount, toast } = await mountConsumers();
    const old = deferred<RepositoryDiscoveryResponse>();
    const kind = name === "Remove" ? "refresh" : "load";
    const transport =
      name === "Remove" ? refreshRepositoryDiscoveryAction : getRepositoryDiscoveryAction;
    vi.mocked(transport).mockReturnValueOnce(old.promise);
    actionTransport(name).mockRejectedValueOnce(new Error("mutation rejected"));
    let oldCall!: Promise<RepositoryDiscoveryResponse | null>;
    act(() => {
      oldCall = repositoryDiscoveryCoordinator[kind]("workspace");
    });
    try {
      await act(async () => {
        await perform(result.current.actions, name);
      });
      expect(result.current.actions).toMatchObject({
        isMutating: false,
        isConfirmingHomeDiscovery: false,
      });
      expect(toast).toHaveBeenCalledWith(
        expect.objectContaining({ description: "mutation rejected" }),
      );
      expect(transport).toHaveBeenCalledTimes(name === "Remove" ? 1 : 2);
      const accepted = snapshot("/still-valid");
      await act(async () => {
        old.resolve(accepted);
        await oldCall;
      });
      expect(result.current.office).toEqual(accepted.repositories);
      await act(async () => {
        await perform(result.current.actions, name);
      });
      expect(actionTransport(name)).toHaveBeenCalledTimes(2);
      expect(result.current.actions.isMutating).toBe(false);
    } finally {
      await act(async () => {
        old.resolve(snapshot("/cleanup"));
        await oldCall;
      });
      unmount();
    }
  },
);

it("manual Refresh joins current work without fencing it", async () => {
  const { result, unmount } = await mountConsumers();
  const old = deferred<RepositoryDiscoveryResponse>();
  vi.mocked(refreshRepositoryDiscoveryAction).mockReturnValueOnce(old.promise);
  let oldCall!: Promise<RepositoryDiscoveryResponse | null>;
  let action!: Promise<void>;
  act(() => {
    oldCall = repositoryDiscoveryCoordinator.refresh("workspace");
  });
  await act(async () => {
    action = result.current.actions.refreshDiscovery();
    await Promise.resolve();
  });
  const current = snapshot("/manual");
  await act(async () => {
    old.resolve(current);
    await Promise.all([oldCall, action]);
  });
  expect(refreshRepositoryDiscoveryAction).toHaveBeenCalledTimes(1);
  expect(result.current.office).toEqual(current.repositories);
  expect(result.current.actions.isMutating).toBe(false);
  unmount();
});

it("serializes Home confirmation and competing root actions through synchronization", async () => {
  const { result, unmount } = await mountConsumers();
  const confirm = deferred<Awaited<ReturnType<typeof confirmHomeDesktopDiscoveryAction>>>();
  const fresh = deferred<RepositoryDiscoveryResponse>();
  vi.mocked(confirmHomeDesktopDiscoveryAction).mockReturnValueOnce(confirm.promise);
  vi.mocked(getRepositoryDiscoveryAction).mockReturnValueOnce(fresh.promise);
  let first!: Promise<void>;
  act(() => {
    first = result.current.actions.handleConfirmHomeDiscovery();
  });
  try {
    await act(async () => {
      await result.current.actions.handleConfirmHomeDiscovery();
      await result.current.actions.handleChooseDiscoveryRoot("/blocked");
      await result.current.actions.handleRemoveDiscoveryRoot("/blocked");
    });
    expect(confirmHomeDesktopDiscoveryAction).toHaveBeenCalledTimes(1);
    expect(addDesktopDiscoveryRootAction).not.toHaveBeenCalled();
    expect(removeDesktopDiscoveryRootAction).not.toHaveBeenCalled();
    await act(async () => {
      confirm.resolve({ id: "home", path: "/home", display_path: "~", state: "connected" });
      await Promise.resolve();
    });
    expect(result.current.actions.isMutating).toBe(true);
    await act(async () => {
      fresh.resolve(snapshot("/home"));
      await first;
    });
    await act(async () => {
      await result.current.actions.handleRemoveDiscoveryRoot("/home");
    });
    expect(removeDesktopDiscoveryRootAction).toHaveBeenCalledTimes(1);
    expect(result.current.actions).toMatchObject({
      isMutating: false,
      isConfirmingHomeDiscovery: false,
    });
  } finally {
    await act(async () => {
      confirm.resolve({ id: "cleanup", path: "/home", display_path: "~", state: "connected" });
      fresh.resolve(snapshot("/baseline"));
      await first;
    });
    unmount();
  }
});

it("keeps disabled and null workspace synchronization inert", async () => {
  const { result, unmount, rerender } = renderHook(
    ({ workspace, enabled }) => useRepositoryDiscovery(workspace, enabled),
    { initialProps: { workspace: "A" as string | null, enabled: false } },
  );
  await act(async () => {
    expect(await result.current.synchronizeAfterRootMutation("load")).toBeNull();
  });
  rerender({ workspace: null, enabled: true });
  await act(async () => {
    expect(await result.current.synchronizeAfterRootMutation("refresh")).toBeNull();
  });
  expect(getRepositoryDiscoveryAction).not.toHaveBeenCalled();
  expect(refreshRepositoryDiscoveryAction).not.toHaveBeenCalled();
  unmount();
});
