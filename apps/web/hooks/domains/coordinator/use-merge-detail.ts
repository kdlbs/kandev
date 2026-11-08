"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  isStoredProposal,
  type MergeProposal,
  type WireProposal,
} from "@/lib/api/domains/coordinator-api";
import {
  checkMergeNow,
  getMergeProposal,
  releaseMerge,
  type MergeDetail,
} from "@/lib/api/domains/coordinator-merge-api";
import { useProposalsStore } from "./use-proposals";

export type MergeDetailStatus = "loading" | "ready" | "error";
export type MergeActionError = "check" | "release";

/**
 * The detail read of one merge proposal (one uncached provider read per open),
 * re-read whenever the proposal's status or revision changes, plus Check now and
 * Release. An action in flight ignores a second press.
 */
export function useMergeDetail(workspaceId: string, coordinatorId: string, p: MergeProposal) {
  const [detail, setDetail] = useState<MergeDetail | null>(p.merge ?? null);
  const [status, setStatus] = useState<MergeDetailStatus>(p.merge ? "ready" : "loading");
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<MergeActionError | null>(null);
  const sequenceRef = useRef(0);
  const lockRef = useRef(false);

  const reload = useCallback(async () => {
    const sequence = ++sequenceRef.current;
    try {
      const next = await getMergeProposal(workspaceId, coordinatorId, p.id);
      if (sequence !== sequenceRef.current) return;
      setDetail(next.merge ?? null);
      setStatus("ready");
    } catch {
      if (sequence !== sequenceRef.current) return;
      setStatus("error");
    }
  }, [workspaceId, coordinatorId, p.id]);

  useEffect(() => {
    setStatus((current) => (current === "ready" ? current : "loading"));
    void reload();
    return () => {
      sequenceRef.current += 1;
    };
    // The detail is re-read when the row's status or revision moves.
  }, [reload, p.status, p.updated_at]);

  const apply = useCallback(
    (proposal: WireProposal) => {
      if (!isStoredProposal(proposal)) return;
      const store = useProposalsStore.getState();
      const seq = store.takeProposalTicket(coordinatorId);
      store.applyProposalResult(coordinatorId, p.id, seq, { kind: "success", proposal });
    },
    [coordinatorId, p.id],
  );

  const act = useCallback(
    async (kind: MergeActionError, run: () => Promise<WireProposal>) => {
      if (lockRef.current) return;
      lockRef.current = true;
      setBusy(true);
      setActionError(null);
      try {
        apply(await run());
        await reload();
      } catch {
        setActionError(kind);
      } finally {
        lockRef.current = false;
        setBusy(false);
      }
    },
    [apply, reload],
  );

  const checkNow = useCallback(
    () => act("check", () => checkMergeNow(workspaceId, coordinatorId, p.id)),
    [act, workspaceId, coordinatorId, p.id],
  );
  const release = useCallback(
    () => act("release", () => releaseMerge(workspaceId, coordinatorId, p.id)),
    [act, workspaceId, coordinatorId, p.id],
  );

  return { detail, status, busy, actionError, reload, checkNow, release };
}
