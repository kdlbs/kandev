import type { WorkflowSnapshot } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import { SharedResourceReads, storeReadScopeIdentity } from "./shared-resource-reads";

export type WorkflowSnapshotReads = SharedResourceReads<WorkflowSnapshot>;

type WorkflowSnapshotReadOwner = { identity: string; reads: WorkflowSnapshotReads };
const owners = new WeakMap<StoreApi<AppState>, WorkflowSnapshotReadOwner>();

export function getWorkflowSnapshotReads(store: StoreApi<AppState>): WorkflowSnapshotReads {
  const identity = storeReadScopeIdentity(store);
  let owner = owners.get(store);
  if (!owner || owner.identity !== identity) {
    owner?.reads.dispose();
    const nextOwner: WorkflowSnapshotReadOwner = {
      identity,
      reads: new SharedResourceReads<WorkflowSnapshot>(
        () => owners.get(store) === nextOwner && storeReadScopeIdentity(store) === identity,
      ),
    };
    owner = nextOwner;
    owners.set(store, owner);
  }
  return owner.reads;
}
