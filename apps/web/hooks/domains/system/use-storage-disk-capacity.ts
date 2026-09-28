import { useCallback, useRef } from "react";
import { fetchStorageDisk } from "@/lib/api/domains/system-api";
import type { StorageDiskCapacityResponse, StorageTemporaryDiskCapacity } from "@/lib/types/system";
import type { StorageSection } from "./use-storage-sections";

type LoadSection = <T>(
  section: StorageSection,
  request: () => Promise<T>,
  commit: (value: T) => void,
) => Promise<void>;
type SetStoredDisk = (disk: StorageDiskCapacityResponse | null, identity: string | null) => void;

export function mergeStorageDiskCapacity(
  previous: StorageDiskCapacityResponse | null,
  incoming: StorageDiskCapacityResponse,
): StorageDiskCapacityResponse {
  if (!previous || previous.path !== incoming.path) return incoming;
  const home =
    incoming.available || !previous.available
      ? { ...incoming, stale: false }
      : { ...previous, warning: incoming.warning, stale: true };
  if (!Array.isArray(incoming.temporary_roots)) {
    return { ...home, temporary_roots: undefined };
  }
  const previousRoots = previous.temporary_roots ?? [];
  if (incoming.temporary_roots.length === 0 && incoming.temporary_roots_warning) {
    return {
      ...home,
      temporary_roots: previousRoots.map((root) =>
        root.available ? { ...root, stale: true } : root,
      ),
      temporary_roots_warning: incoming.temporary_roots_warning,
    };
  }
  const temporaryRoots = incoming.temporary_roots.map((root): StorageTemporaryDiskCapacity => {
    if (root.available) return { ...root, stale: false };
    const lastGood = previousRoots.find(
      (candidate) =>
        candidate.available &&
        (candidate.path === root.path || candidate.requested_path === root.requested_path),
    );
    if (!lastGood) return root;
    return {
      ...lastGood,
      requested_path: root.requested_path,
      path: root.path,
      aliases: root.aliases,
      warning: root.warning,
      stale: true,
    };
  });
  return { ...home, temporary_roots: temporaryRoots };
}

function staleDiskCapacity(previous: StorageDiskCapacityResponse | null) {
  if (!previous) return previous;
  return {
    ...previous,
    stale: true,
    temporary_roots: previous.temporary_roots?.map((root) =>
      root.available ? { ...root, stale: true } : root,
    ),
  };
}

export function useStorageDiskCapacity(
  storedDisk: StorageDiskCapacityResponse | null,
  storedDiskIdentity: string | null,
  authIdentity: string,
  setDiskInStore: SetStoredDisk,
  loadSection: LoadSection,
) {
  const diskIdentity = `${typeof window === "undefined" ? "" : window.location.origin}\u0000${authIdentity}`;
  const currentDisk = storedDiskIdentity === diskIdentity ? storedDisk : null;
  const diskSnapshot = useRef<StorageDiskCapacityResponse | null>(currentDisk);
  diskSnapshot.current = currentDisk;
  const diskRequest = useRef<{ identity: string; promise: Promise<void> } | null>(null);
  const loadDisk = useCallback(() => {
    if (diskRequest.current?.identity === diskIdentity) return diskRequest.current.promise;
    const request = loadSection("disk", fetchStorageDisk, (incoming) => {
      const next = mergeStorageDiskCapacity(diskSnapshot.current, incoming);
      diskSnapshot.current = next;
      setDiskInStore(next, diskIdentity);
    }).catch((error: unknown) => {
      const stale = staleDiskCapacity(diskSnapshot.current);
      if (stale) {
        diskSnapshot.current = stale;
        setDiskInStore(stale, diskIdentity);
      }
      throw error;
    });
    const sharedRequest = request.finally(() => {
      if (diskRequest.current?.promise === sharedRequest) diskRequest.current = null;
    });
    diskRequest.current = { identity: diskIdentity, promise: sharedRequest };
    return sharedRequest;
  }, [diskIdentity, loadSection, setDiskInStore]);
  return { diskIdentity, currentDisk, loadDisk };
}
