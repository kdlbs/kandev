import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import type {
  ArchivedDataBackupChoice,
  ArchivedDataPolicy,
} from "@/lib/types/archived-data-retention";
import type { useArchivedDataRetention } from "./use-archived-data-retention";

export const maxArchivedDays = 3650;
export function validArchivedDays(age: ArchivedDataPolicy["archived_age"]) {
  return Number.isInteger(age.days) && age.days >= 1 && age.days <= maxArchivedDays;
}
const serialize = (value: unknown) => JSON.stringify(value);
// A shorter window brings the cutoff forward, so it removes data sooner than the
// administrator already approved. The cutoff moves later as the window shrinks,
// so a draft cutoff after the saved one is the review trigger.
export function cutoff(now: Date, days: number) {
  return new Date(now.getTime() - days * 24 * 60 * 60 * 1000).getTime();
}
export function needsArchivedBackupReview(
  draft: ArchivedDataPolicy | null,
  saved: ArchivedDataPolicy | undefined,
  now: Date,
) {
  if (!draft?.enabled || !saved) return false;
  if (!saved.enabled) return true;
  return (
    cutoff(now, draft.archived_age.days) > cutoff(now, saved.archived_age.days) ||
    cutoff(now, draft.cleanup_age.days) > cutoff(now, saved.cleanup_age.days)
  );
}
function invalidAges(draft: ArchivedDataPolicy | null) {
  if (!draft) return true;
  return !validArchivedDays(draft.archived_age) || !validArchivedDays(draft.cleanup_age);
}
function acceptSavedDraft(
  current: ArchivedDataPolicy | null,
  submitted: ArchivedDataPolicy,
  saved: ArchivedDataPolicy,
) {
  if (!current || serialize(current) === serialize(submitted)) return saved;
  return { ...current, revision: saved.revision };
}
// rebaseArchivedDraft carries a local edit onto a newer saved policy. Only the
// fields the administrator changed against the previous saved policy are kept;
// every other field, and the revision, come from the new saved policy. A
// preparation that enables the policy, or a cancellation that advances the
// revision, therefore cannot leave a stale flag or revision in the draft.
export function rebaseArchivedDraft(
  current: ArchivedDataPolicy | null,
  previous: ArchivedDataPolicy | null,
  saved: ArchivedDataPolicy,
): ArchivedDataPolicy {
  if (!current || !previous || serialize(current) === serialize(previous)) return saved;
  const next: ArchivedDataPolicy = { ...saved };
  if (current.enabled !== previous.enabled) next.enabled = current.enabled;
  if (serialize(current.archived_age) !== serialize(previous.archived_age))
    next.archived_age = current.archived_age;
  if (serialize(current.cleanup_age) !== serialize(previous.cleanup_age))
    next.cleanup_age = current.cleanup_age;
  return serialize(next) === serialize(current) ? current : next;
}
function validationKey(canEdit: boolean, invalid: boolean, needsChoice: boolean, choice: string) {
  if (!canEdit) return "system:archivedData.adminOnly";
  if (invalid) return "system:archivedData.invalidDays";
  if (needsChoice && !choice) return "system:archivedData.chooseBackup";
  return null;
}

export function useArchivedDataRetentionDraft(
  remote: ReturnType<typeof useArchivedDataRetention>,
  admin: boolean,
) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<ArchivedDataPolicy | null>(null);
  const [choice, setChoice] = useState<ArchivedDataBackupChoice | "">("");
  const saved = remote.status?.policy;
  const baseline = useRef<ArchivedDataPolicy | null>(null);
  // A draft belongs to the backend identity it was edited against; a new
  // identity starts from that backend's saved policy.
  const scope = remote.captureScope();
  const scopeKey = JSON.stringify([scope.identityKey, scope.generation]);
  useLayoutEffect(() => {
    baseline.current = null;
    setDraft(null);
    setChoice("");
  }, [scopeKey]);
  useEffect(() => {
    if (!saved) return;
    const previous = baseline.current;
    setDraft((current) => rebaseArchivedDraft(current, previous, saved));
    baseline.current = saved;
  }, [saved]);
  const dirty = Boolean(draft && saved && serialize(draft) !== serialize(saved));
  const needsChoice = needsArchivedBackupReview(draft, saved, new Date());
  const invalid = invalidAges(draft);
  const canEdit = admin && Boolean(remote.status?.supported);
  const validation = validationKey(canEdit, invalid, needsChoice, choice);
  const invalidReason = validation ? t(validation) : undefined;
  useSettingsSaveContributor({
    id: "system:archived-data-retention",
    order: 27,
    revision: serialize({ draft, choice }),
    isDirty: dirty,
    canSave: canEdit && !remote.pending && !invalid && (!needsChoice || Boolean(choice)),
    invalidReason,
    save: async () => {
      if (!draft || invalid || !canEdit || (needsChoice && !choice)) return;
      const submitted = draft;
      const writerScope = remote.captureScope();
      const next = await remote.save(
        {
          ...submitted,
          ...(needsChoice && choice ? { backup_choice: choice } : {}),
        },
        { backupChoiceAttempt: needsChoice && choice === "backup" },
      );
      if (!remote.isCurrentScope(writerScope)) return;
      setDraft((current) => acceptSavedDraft(current, submitted, next.policy));
      setChoice("");
    },
    discard: () => {
      if (saved) setDraft(saved);
      setChoice("");
      void remote.refresh();
    },
  });
  const setEnabled = (enabled: boolean) =>
    setDraft((current) => {
      if (!current) return current;
      const next: ArchivedDataPolicy = { ...current, enabled };
      // Turning the policy off must not discard an in-progress window edit, and an
      // invalid window falls back to the saved one rather than blocking the save.
      if (!enabled) {
        if (!validArchivedDays(next.archived_age) && saved)
          next.archived_age = { ...saved.archived_age };
        if (!validArchivedDays(next.cleanup_age) && saved)
          next.cleanup_age = { ...saved.cleanup_age };
      }
      return next;
    });
  return {
    setEnabled,
    draft,
    setDraft,
    choice,
    setChoice,
    needsChoice,
    dirty,
    invalid,
    canEdit,
    invalidReason,
  };
}
