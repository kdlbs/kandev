import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import {
  createBackupListQueryKey,
  invalidateBackupList,
  type BackupListQueryIdentity,
} from "./backup-list-query";

const IDENTITY: BackupListQueryIdentity = {
  apiBaseUrl: "https://backend.example/api///",
  bootId: "boot-1",
  authMode: "enabled",
  authenticated: true,
  userId: "user-1",
};

describe("backup-list query identity", () => {
  it("uses the normalized full backend, process, and auth identity", () => {
    expect(createBackupListQueryKey(IDENTITY)).toEqual([
      "system",
      "backups",
      "https://backend.example/api",
      "boot-1",
      "enabled",
      true,
      "user-1",
    ]);
    expect(
      createBackupListQueryKey({ ...IDENTITY, authenticated: false, userId: null }),
    ).not.toEqual(createBackupListQueryKey(IDENTITY));
  });
});

describe("backup-list invalidation", () => {
  it("refreshes an active exact identity and leaves an inactive entry stale", async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: Infinity } },
    });
    const queryKey = createBackupListQueryKey(IDENTITY);
    const otherKey = createBackupListQueryKey({ ...IDENTITY, userId: "user-2" });
    const fetchList = vi.fn(async () => [{ name: "manual-1.db" }]);
    const otherFetch = vi.fn(async () => [{ name: "manual-other.db" }]);
    const observer = new QueryObserver(queryClient, { queryKey, queryFn: fetchList });
    const unsubscribe = observer.subscribe(() => undefined);
    const otherObserver = new QueryObserver(queryClient, {
      queryKey: otherKey,
      queryFn: otherFetch,
    });
    const unsubscribeOther = otherObserver.subscribe(() => undefined);

    try {
      await observer.refetch();
      await otherObserver.refetch();
      fetchList.mockClear();
      otherFetch.mockClear();

      await invalidateBackupList(queryClient, IDENTITY);

      expect(fetchList).toHaveBeenCalledOnce();
      expect(otherFetch).not.toHaveBeenCalled();
      unsubscribe();
      fetchList.mockClear();

      await invalidateBackupList(queryClient, IDENTITY);

      expect(fetchList).not.toHaveBeenCalled();
      expect(queryClient.getQueryState(queryKey)?.isInvalidated).toBe(true);
    } finally {
      unsubscribe();
      unsubscribeOther();
      queryClient.clear();
    }
  });
});
