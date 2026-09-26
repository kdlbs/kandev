"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "@/lib/toast/sonner";
import * as officeApi from "@/lib/api/domains/office-api";
import type { SyncDiff } from "@/lib/api/domains/office-api";
import { useTranslation } from "react-i18next";

// Catalog keys, not messages — module scope freezes a `t()` at the boot locale.
const MSG_LOAD_FAIL = "office:failedToLoadDiffs";
const MSG_IMPORT_FAIL = "office:failedToImportFromFilesystem";
const MSG_EXPORT_FAIL = "office:failedToExportToFilesystem";

export function useSyncState(activeWorkspaceId: string) {
  const { t } = useTranslation();
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
        officeApi.getIncomingDiff(activeWorkspaceId),
        officeApi.getOutgoingDiff(activeWorkspaceId),
      ]);
      setIncoming(inRes.diff);
      setOutgoing(outRes.diff);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t(MSG_LOAD_FAIL));
    } finally {
      setLoading(false);
    }
  }, [activeWorkspaceId, t]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const applyIncoming = useCallback(async () => {
    if (!activeWorkspaceId) return;
    setApplyingIn(true);
    try {
      const res = await officeApi.applyIncomingSync(activeWorkspaceId);
      toast.success(
        t("office:importedFromFilesystemCreatedUpdated", {
          created: res.result.created_count,
          updated: res.result.updated_count,
        }),
      );
      if (res.result.warnings?.length) {
        toast.warning(res.result.warnings.join("\n"));
      }
      await refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t(MSG_IMPORT_FAIL));
    } finally {
      setApplyingIn(false);
    }
  }, [activeWorkspaceId, refresh, t]);

  const applyOutgoing = useCallback(async () => {
    if (!activeWorkspaceId) return;
    setApplyingOut(true);
    try {
      await officeApi.applyOutgoingSync(activeWorkspaceId);
      toast.success(t("office:exportedToFilesystem"));
      await refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t(MSG_EXPORT_FAIL));
    } finally {
      setApplyingOut(false);
    }
  }, [activeWorkspaceId, refresh, t]);

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
