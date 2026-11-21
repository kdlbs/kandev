"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  getCoordinatorSettings,
  putCoordinatorSettings,
  type ControlSetting,
  type CoordinatorSettings,
} from "@/lib/api/domains/coordinator-api";
import {
  mergeErrorCode,
  refreshMergeProtection,
  type MergeRepo,
  type MergeRepoChange,
  type ProtectionObservation,
} from "@/lib/api/domains/coordinator-merge-api";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { toast } from "@/lib/toast/sonner";
import { useWebSocketClient } from "@/lib/ws/connection";
import {
  buildMergeChanges,
  isMergeDirty,
  markReplacePrincipal,
  pruneDraft,
  sameStoredPolicies,
  setPolicy,
  type MergeDraft,
} from "@/lib/coordinators/merge-draft";

export type MergeLoadStatus = "loading" | "ready" | "error" | "off";

type Stored = { repos: MergeRepo[]; protection: ProtectionObservation[]; revision: number };

/** The closed reason a merge save failed; null while there is none. */
export type MergeSaveError =
  | "conflict"
  | "principal_valid"
  | "merge_unavailable"
  | "repository_unknown"
  | "failed";

function storedOf(settings: CoordinatorSettings): Stored | null {
  if (!settings.merge) return null;
  return {
    repos: settings.merge.repositories,
    protection: settings.merge.protection,
    revision: settings.policy_revision,
  };
}

function saveErrorOf(error: unknown): MergeSaveError {
  const code = mergeErrorCode(error);
  if (code === "revision_conflict" || code === "revision_required") return "conflict";
  if (code === "principal_valid" || code === "merge_unavailable" || code === "repository_unknown") {
    return code;
  }
  return "failed";
}

type Params = { workspaceId: string; coordinatorId: string; canManage: boolean; enabled: boolean };

/**
 * The Merge section's one read and one save. The draft holds only unsaved edits, so a
 * re-read never overwrites an edit; a save lists only the repositories that differ and
 * carries the read policy revision.
 */
// eslint-disable-next-line max-lines-per-function -- one hook owns the read, the draft, the revisioned save and the protection refresh
export function useMergeSettings({ workspaceId, coordinatorId, canManage, enabled }: Params) {
  const { t } = useTranslation();
  const [stored, setStored] = useState<Stored | null>(null);
  const [draft, setDraft] = useState<MergeDraft>({});
  const [status, setStatus] = useState<MergeLoadStatus>("loading");
  const [saveError, setSaveError] = useState<MergeSaveError | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshFailed, setRefreshFailed] = useState(false);
  const storedRef = useRef(stored);
  storedRef.current = stored;
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const sequenceRef = useRef(0);

  const adopt = useCallback((next: Stored) => {
    storedRef.current = next;
    setStored(next);
    const pruned = pruneDraft(draftRef.current, next.repos);
    draftRef.current = pruned;
    setDraft(pruned);
  }, []);

  const readSettings = useCallback(async (): Promise<Stored | null> => {
    return storedOf(await getCoordinatorSettings(workspaceId, coordinatorId));
  }, [workspaceId, coordinatorId]);

  const reload = useCallback(() => {
    const sequence = ++sequenceRef.current;
    readSettings()
      .then((next) => {
        if (sequence !== sequenceRef.current) return;
        if (!next) setStatus("off");
        else {
          adopt(next);
          setStatus("ready");
        }
      })
      .catch(() => {
        if (sequence !== sequenceRef.current || storedRef.current) return;
        setStatus("error");
      });
  }, [readSettings, adopt]);

  const retry = useCallback(() => {
    setStatus("loading");
    reload();
  }, [reload]);

  useEffect(() => {
    if (!enabled) return;
    storedRef.current = null;
    draftRef.current = {};
    setStored(null);
    setDraft({});
    setSaveError(null);
    setStatus("loading");
    reload();
    return () => {
      sequenceRef.current += 1;
    };
  }, [enabled, reload]);

  const wsClient = useWebSocketClient();
  useEffect(() => {
    if (!enabled || !wsClient) return;
    return wsClient.on("coordinator.updated", (message) => {
      const payload = message.payload;
      if (payload.workspace_id !== workspaceId || payload.coordinator_id !== coordinatorId) return;
      reload();
    });
  }, [enabled, wsClient, workspaceId, coordinatorId, reload]);

  const edit = useCallback((change: (current: MergeDraft) => MergeDraft) => {
    draftRef.current = change(draftRef.current);
    setDraft(draftRef.current);
    setSaveError(null);
  }, []);

  const choosePolicy = useCallback(
    (repo: MergeRepo, policy: ControlSetting) => edit((d) => setPolicy(d, repo, policy)),
    [edit],
  );
  const replacePrincipal = useCallback(
    (repo: MergeRepo) => edit((d) => markReplacePrincipal(d, repo)),
    [edit],
  );

  const put = (revision: number, changes: MergeRepoChange[]) =>
    putCoordinatorSettings(workspaceId, coordinatorId, {
      policy_revision: revision,
      merge: { repositories: changes },
    });

  // A conflict is retried once when a concurrent save of another section moved the
  // revision but left the listed repositories as they were read.
  const putWithRevisionRetry = async (base: Stored, changes: MergeRepoChange[]) => {
    try {
      return await put(base.revision, changes);
    } catch (error) {
      if (saveErrorOf(error) !== "conflict") throw error;
      const fresh = await readSettings();
      const ids = changes.map((c) => c.repository_id);
      if (!fresh || !sameStoredPolicies(ids, base.repos, fresh.repos)) {
        if (fresh) adopt(fresh);
        throw error;
      }
      return put(fresh.revision, changes);
    }
  };

  const save = async () => {
    const base = storedRef.current;
    if (!base) return;
    const changes = buildMergeChanges(draftRef.current, base.repos);
    if (changes.length === 0) return;
    setSaveError(null);
    sequenceRef.current += 1;
    try {
      const next = storedOf(await putWithRevisionRetry(base, changes));
      sequenceRef.current += 1;
      if (next) adopt(next);
      else reload();
    } catch (error) {
      setSaveError(saveErrorOf(error));
      if (saveErrorOf(error) === "failed") toast.error(t("coordinator:mergeSaveFailed"));
      throw error;
    }
  };

  const dirty = canManage && !!stored && isMergeDirty(draft, stored.repos);
  useSettingsSaveContributor({
    id: "coordinator-merge",
    revision: JSON.stringify(draft),
    isDirty: dirty,
    save,
    discard: () => {
      draftRef.current = {};
      setDraft({});
      setSaveError(null);
    },
  });

  const refreshProtection = useCallback(async () => {
    setRefreshing(true);
    setRefreshFailed(false);
    try {
      const { protection } = await refreshMergeProtection(workspaceId, coordinatorId);
      const current = storedRef.current;
      if (current) adopt({ ...current, protection });
    } catch {
      setRefreshFailed(true);
    } finally {
      setRefreshing(false);
    }
  }, [workspaceId, coordinatorId, adopt]);

  return {
    status,
    repos: stored?.repos ?? [],
    protection: stored?.protection ?? [],
    draft,
    saveError,
    refreshing,
    refreshFailed,
    retry,
    choosePolicy,
    replacePrincipal,
    refreshProtection,
  };
}
