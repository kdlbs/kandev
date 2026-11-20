import { useCallback, useEffect, useRef, useState } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import {
  fetchMessageQueueSettings,
  updateMessageQueueSettings,
} from "@/lib/api/domains/settings-api";
import type { MessageQueueSettingsResponse, MessageQueueSettingsSource } from "@/lib/types/system";

const ENVIRONMENT_VARIABLE = "KANDEV_QUEUE_MAX_PER_SESSION";
/** Parses the max-per-session draft text into a non-negative safe integer,
 * or `null` when the text isn't a valid whole number. */
function parseMaximum(value: string): number | null {
  const trimmed = value.trim();
  if (!/^\d+$/.test(trimmed)) return null;
  const parsed = Number(trimmed);
  return Number.isSafeInteger(parsed) ? parsed : null;
}
export function queueCapacityLockMessage(
  t: TFunction,
  source: MessageQueueSettingsSource | undefined,
): string {
  if (source === "configuration") return t("system:messageQueueConfigurationLocked");
  return t("system:messageQueueEnvironmentLocked", { variable: ENVIRONMENT_VARIABLE });
}

type QueueSettingsValidationOptions = {
  parsed: number | null;
  isAdmin: boolean;
  isLocked: boolean;
  isMaxDirty: boolean;
  lockSource: MessageQueueSettingsSource | undefined;
};

function queueSettingsInvalidReason(
  t: TFunction,
  { parsed, isAdmin, isLocked, isMaxDirty, lockSource }: QueueSettingsValidationOptions,
): string | undefined {
  if (parsed === null) return t("system:messageQueueValidation");
  if (!isAdmin) return t("system:messageQueueAdminOnly");
  if (isLocked && isMaxDirty) return queueCapacityLockMessage(t, lockSource);
  return undefined;
}
/** Loads the persisted snapshot and owns the editable queue-setting drafts.
 * Split out of
 * `useMessageQueueSettingsDraft` so neither function needs to grow past the
 * project's per-function line limit as the page gains fields. */
function useMessageQueueSettingsLoad() {
  const [snapshot, setSnapshot] = useState<MessageQueueSettingsResponse | null>(null);
  const [draft, setDraft] = useState("");
  const [mergeDraft, setMergeDraft] = useState(true);
  const [autoMergeDraft, setAutoMergeDraft] = useState(true);
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const loadVersion = useRef(0);

  const reload = useCallback(async () => {
    const version = ++loadVersion.current;
    setLoading(true);
    setLoadFailed(false);
    try {
      const response = await fetchMessageQueueSettings();
      if (version !== loadVersion.current) return;
      setSnapshot(response);
      setDraft(String(response.settings.max_per_session));
      setMergeDraft(response.settings.merge_enabled);
      setAutoMergeDraft(response.settings.auto_merge_enabled);
    } catch {
      if (version === loadVersion.current) setLoadFailed(true);
    } finally {
      if (version === loadVersion.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
    return () => {
      loadVersion.current += 1;
    };
  }, [reload]);

  return {
    snapshot,
    setSnapshot,
    draft,
    setDraft,
    mergeDraft,
    setMergeDraft,
    autoMergeDraft,
    setAutoMergeDraft,
    loading,
    loadFailed,
    reload,
  };
}

type QueueSettingsContributorOptions = {
  load: ReturnType<typeof useMessageQueueSettingsLoad>;
  parsed: number | null;
  isMaxDirty: boolean;
  isMergeDirty: boolean;
  isAutoMergeDirty: boolean;
  isAdmin: boolean;
  isLocked: boolean;
  invalidReason: string | undefined;
  onSaveFailed: (failed: boolean) => void;
};

/** Registers all queue drafts as one atomic settings-page contributor. */
function useQueueSettingsContributor({
  load,
  parsed,
  isMaxDirty,
  isMergeDirty,
  isAutoMergeDirty,
  isAdmin,
  isLocked,
  invalidReason,
  onSaveFailed,
}: QueueSettingsContributorOptions) {
  const {
    snapshot,
    setSnapshot,
    draft,
    setDraft,
    mergeDraft,
    setMergeDraft,
    autoMergeDraft,
    setAutoMergeDraft,
  } = load;
  const baseline = snapshot?.settings.max_per_session;
  const mergeBaseline = snapshot?.settings.merge_enabled;
  const autoMergeBaseline = snapshot?.settings.auto_merge_enabled;
  const isDirty = isMaxDirty || isMergeDirty || isAutoMergeDirty;
  useSettingsSaveContributor({
    id: "system-message-queue",
    revision: `${draft}:${mergeDraft}:${autoMergeDraft}`,
    isDirty,
    canSave: parsed !== null && isAdmin && (!isMaxDirty || !isLocked),
    invalidReason,
    save: async () => {
      if (parsed === null || !isAdmin || (isMaxDirty && isLocked)) {
        throw new Error(invalidReason);
      }
      const submitted = draft;
      const submittedMerge = mergeDraft;
      const submittedAutoMerge = autoMergeDraft;
      onSaveFailed(false);
      try {
        const response = await updateMessageQueueSettings({
          ...(isMaxDirty ? { max_per_session: parsed } : {}),
          ...(isMergeDirty ? { merge_enabled: mergeDraft } : {}),
          ...(isAutoMergeDirty ? { auto_merge_enabled: autoMergeDraft } : {}),
        });
        setSnapshot(response);
        setDraft((current) =>
          current === submitted ? String(response.settings.max_per_session) : current,
        );
        setMergeDraft((current) =>
          current === submittedMerge ? response.settings.merge_enabled : current,
        );
        setAutoMergeDraft((current) =>
          current === submittedAutoMerge ? response.settings.auto_merge_enabled : current,
        );
      } catch (error) {
        onSaveFailed(true);
        throw error;
      }
    },
    discard: () => {
      if (baseline !== undefined) setDraft(String(baseline));
      if (mergeBaseline !== undefined) setMergeDraft(mergeBaseline);
      if (autoMergeBaseline !== undefined) setAutoMergeDraft(autoMergeBaseline);
      onSaveFailed(false);
    },
  });
}

/** Derives dirty, validation, and permission state for all queue drafts. */
export function useMessageQueueSettingsDraft() {
  const { t } = useTranslation();
  const isAdmin = useAppStore(
    (state) => state.auth.user?.role === undefined || state.auth.user.role === "admin",
  );
  const [saveFailed, setSaveFailed] = useState(false);
  const load = useMessageQueueSettingsLoad();
  const { snapshot, draft, mergeDraft, autoMergeDraft } = load;
  const parsed = parseMaximum(draft);
  const baseline = snapshot?.settings.max_per_session;
  const mergeBaseline = snapshot?.settings.merge_enabled;
  const autoMergeBaseline = snapshot?.settings.auto_merge_enabled;
  const isMaxDirty = baseline !== undefined && draft !== String(baseline);
  const isMergeDirty = mergeBaseline !== undefined && mergeDraft !== mergeBaseline;
  const isAutoMergeDirty = autoMergeBaseline !== undefined && autoMergeDraft !== autoMergeBaseline;
  const isLocked = snapshot?.effective.locked === true;
  const lockSource = snapshot?.effective.source;
  const invalidReason = queueSettingsInvalidReason(t, {
    parsed,
    isAdmin,
    isLocked,
    isMaxDirty,
    lockSource,
  });

  useQueueSettingsContributor({
    load,
    parsed,
    isMaxDirty,
    isMergeDirty,
    isAutoMergeDirty,
    isAdmin,
    isLocked,
    invalidReason,
    onSaveFailed: setSaveFailed,
  });

  return {
    snapshot,
    draft,
    setDraft: load.setDraft,
    mergeDraft,
    setMergeDraft: load.setMergeDraft,
    autoMergeDraft,
    setAutoMergeDraft: load.setAutoMergeDraft,
    loading: load.loading,
    loadFailed: load.loadFailed,
    saveFailed,
    invalidReason,
    isDirty: isMaxDirty || isMergeDirty || isAutoMergeDirty,
    isAdmin,
    isLocked,
    lockSource,
    reload: load.reload,
  };
}
