"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  readRelay,
  type Relay,
  type RelayClarification,
  type RelayPermission,
  type RelayRead,
} from "@/lib/api/domains/coordinator-relay-api";

export type RelayPendingAction = "clarification" | "permission";

/** What an expanded item renders. The kind is fixed at expansion and never follows the task. */
export type RelayRendered =
  | { kind: "clarification"; bundle: RelayClarification }
  | { kind: "permission"; permission: RelayPermission };

type UseRelayItemParams = {
  workspaceId: string;
  coordinatorId: string;
  taskId: string | null;
  pendingAction: RelayPendingAction | undefined;
  /** Phase 3 effective and the viewer is a manager. Anything else makes no relay read at all. */
  enabled: boolean;
  /** Changes whenever the task's status summary changes; each change triggers one event read. */
  refreshKey: string;
};

export type RelayItemState = {
  /** Answer here is offered on the collapsed item. */
  offered: boolean;
  expanded: boolean;
  rendered: RelayRendered | null;
  toggle: () => void;
  markEngaged: () => void;
  /** An outcome ended the answer: collapse, drop the cached result, read once more. */
  finishOutcome: () => void;
};

function permissionRequestId(permission: RelayPermission): string {
  const metadata = permission.message.metadata as { request_id?: string } | undefined;
  return metadata?.request_id ?? "";
}

function permissionPendingId(permission: RelayPermission): string {
  const metadata = permission.message.metadata as { pending_id?: string } | undefined;
  return metadata?.pending_id ?? "";
}

function pick(relay: Relay | null, kind: RelayPendingAction | undefined): RelayRendered | null {
  if (!relay || !kind) return null;
  if (kind === "clarification") {
    return relay.clarification ? { kind, bundle: relay.clarification } : null;
  }
  return relay.permission ? { kind, permission: relay.permission } : null;
}

function isDifferent(current: RelayRendered, next: RelayRendered, strict: boolean): boolean {
  if (current.kind === "clarification" && next.kind === "clarification") {
    return current.bundle.pending_id !== next.bundle.pending_id;
  }
  if (current.kind === "permission" && next.kind === "permission") {
    const requestChanged =
      permissionRequestId(current.permission) !== permissionRequestId(next.permission);
    if (!strict) return requestChanged;
    return (
      requestChanged ||
      permissionPendingId(current.permission) !== permissionPendingId(next.permission)
    );
  }
  return false;
}

/**
 * One Needs you item's relay state: the availability read, the expand read and
 * the held expansion (docs/specs/coordinator/system-design/relay.md
 * "Question card"). One request sequence orders every read of the item; an
 * expand read is authoritative over event reads and is the only read that can
 * collapse an item.
 */
// eslint-disable-next-line max-lines-per-function -- one hook owns the read sequence and the held expansion
export function useRelayItem({
  workspaceId,
  coordinatorId,
  taskId,
  pendingAction,
  enabled,
  refreshKey,
}: UseRelayItemParams): RelayItemState {
  const [cache, setCache] = useState<Relay | null>(null);
  const [rendered, setRendered] = useState<RelayRendered | null>(null);
  const renderedRef = useRef<RelayRendered | null>(null);
  const seq = useRef(0);
  const expandSeq = useRef(0);
  const expandInFlight = useRef(false);
  const engaged = useRef(false);

  const commitRendered = useCallback((next: RelayRendered | null) => {
    renderedRef.current = next;
    setRendered(next);
  }, []);

  const release = useCallback(() => {
    commitRendered(null);
    engaged.current = false;
    expandInFlight.current = false;
    expandSeq.current = 0;
    seq.current += 1;
  }, [commitRendered]);

  const applyExpandRead = useCallback(
    (res: RelayRead) => {
      const current = renderedRef.current;
      if (!current || engaged.current) return;
      if (res.kind !== "ok") {
        setCache(null);
        release();
        return;
      }
      setCache(res.relay);
      const next = pick(res.relay, current.kind);
      if (!next) {
        release();
        return;
      }
      if (isDifferent(current, next, true)) commitRendered(next);
    },
    [commitRendered, release],
  );

  const applyEventRead = useCallback(
    (res: RelayRead) => {
      const current = renderedRef.current;
      if (!current) {
        setCache(res.kind === "ok" ? res.relay : null);
        return;
      }
      if (engaged.current || res.kind !== "ok") return;
      const next = pick(res.relay, current.kind);
      if (next && isDifferent(current, next, false)) commitRendered(next);
    },
    [commitRendered],
  );

  const read = useCallback(
    async (mode: "expand" | "event") => {
      if (!taskId) return;
      const n = ++seq.current;
      if (mode === "expand") {
        expandSeq.current = n;
        expandInFlight.current = true;
      }
      const res = await readRelay(workspaceId, coordinatorId, taskId);
      if (mode === "expand") {
        if (expandSeq.current !== n) return;
        expandInFlight.current = false;
        applyExpandRead(res);
        return;
      }
      if (n !== seq.current || expandInFlight.current) return;
      applyEventRead(res);
    },
    [workspaceId, coordinatorId, taskId, applyExpandRead, applyEventRead],
  );

  useEffect(() => {
    if (!enabled || !taskId || expandInFlight.current) return;
    void read("event");
    // refreshKey re-runs the read when the task's status summary changes.
  }, [enabled, taskId, refreshKey, read]);

  const toggle = useCallback(() => {
    if (renderedRef.current) {
      release();
      return;
    }
    if (!enabled) return;
    const next = pick(cache, pendingAction);
    if (!next) return;
    engaged.current = false;
    commitRendered(next);
    void read("expand");
  }, [enabled, cache, pendingAction, commitRendered, release, read]);

  const markEngaged = useCallback(() => {
    engaged.current = true;
  }, []);

  const finishOutcome = useCallback(() => {
    release();
    setCache(null);
    if (enabled) void read("event");
  }, [enabled, release, read]);

  const offered = enabled && pick(cache, pendingAction) !== null;
  return { offered, expanded: rendered !== null, rendered, toggle, markEngaged, finishOutcome };
}
