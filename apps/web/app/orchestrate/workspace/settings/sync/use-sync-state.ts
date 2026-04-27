"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import * as orchestrateApi from "@/lib/api/domains/orchestrate-api";
import type { SyncDiff } from "@/lib/api/domains/orchestrate-api";

const MSG_LOAD_FAIL = "Failed to load diffs";
const MSG_IMPORT_FAIL = "Failed to import from filesystem";
const MSG_EXPORT_FAIL = "Failed to export to filesystem";

export function useSyncState(activeWorkspaceId: string) {
  const [incoming, setIncoming] = useState<SyncDiff | null>(null);
  const [outgoing, setOutgoing] = useState<SyncDiff | null>(null);
  const [loading, setLoading] = useState(false);
  const [applyingIn, setApplyingIn] = useState(false);
  const [applyingOut, setApplyingOut] = useState(false);

  const refresh = useCallback(async () => {
    if (!activeWorkspaceId) return;
    setLoading(true);
    try {
      const [inRes, outRes] = await Promise.all([
        orchestrateApi.getIncomingDiff(activeWorkspaceId),
        orchestrateApi.getOutgoingDiff(activeWorkspaceId),
      ]);
      setIncoming(inRes.diff);
      setOutgoing(outRes.diff);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : MSG_LOAD_FAIL);
    } finally {
      setLoading(false);
    }
  }, [activeWorkspaceId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const applyIncoming = useCallback(async () => {
    if (!activeWorkspaceId) return;
    setApplyingIn(true);
    try {
      const res = await orchestrateApi.applyIncomingSync(activeWorkspaceId);
      toast.success(
        `Imported from filesystem (created ${res.result.created_count}, updated ${res.result.updated_count})`,
      );
      await refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : MSG_IMPORT_FAIL);
    } finally {
      setApplyingIn(false);
    }
  }, [activeWorkspaceId, refresh]);

  const applyOutgoing = useCallback(async () => {
    if (!activeWorkspaceId) return;
    setApplyingOut(true);
    try {
      await orchestrateApi.applyOutgoingSync(activeWorkspaceId);
      toast.success("Exported to filesystem");
      await refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : MSG_EXPORT_FAIL);
    } finally {
      setApplyingOut(false);
    }
  }, [activeWorkspaceId, refresh]);

  return {
    incoming,
    outgoing,
    loading,
    applyingIn,
    applyingOut,
    refresh,
    applyIncoming,
    applyOutgoing,
  };
}
