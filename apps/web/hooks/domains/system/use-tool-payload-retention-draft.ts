import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import type {
  ToolPayloadAge,
  ToolPayloadBackupChoice,
  ToolPayloadPolicy,
} from "@/lib/types/tool-payload-retention";
import type { useToolPayloadRetention } from "./use-tool-payload-retention";

export const sameAge = (a: ToolPayloadAge, b: ToolPayloadAge) =>
  a.unit === b.unit && a.value === b.value;
export function validAge(age: ToolPayloadAge) {
  return (
    Number.isInteger(age.value) && age.value >= 1 && age.value <= (age.unit === "weeks" ? 520 : 120)
  );
}
function cutoff(age: ToolPayloadAge) {
  const date = new Date();
  if (age.unit === "weeks") date.setUTCDate(date.getUTCDate() - age.value * 7);
  else {
    const day = date.getUTCDate();
    date.setUTCDate(1);
    date.setUTCMonth(date.getUTCMonth() - age.value);
    const last = new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth() + 1, 0)).getUTCDate();
    date.setUTCDate(Math.min(day, last));
  }
  return date.getTime();
}
const serialize = (value: unknown) => JSON.stringify(value);

function needsBackupReview(draft: ToolPayloadPolicy | null, saved?: ToolPayloadPolicy) {
  if (!draft?.enabled || !saved) return false;
  if (!saved.enabled) return true;
  return !sameAge(draft.age, saved.age) && cutoff(draft.age) > cutoff(saved.age);
}
function acceptSavedDraft(
  current: ToolPayloadPolicy | null,
  submitted: ToolPayloadPolicy,
  saved: ToolPayloadPolicy,
) {
  if (!current || serialize(current) === serialize(submitted)) return saved;
  return { ...current, revision: saved.revision };
}
function validationKey(canEdit: boolean, invalid: boolean, needsChoice: boolean, choice: string) {
  if (!canEdit) return "system:toolPayload.adminOnly";
  if (invalid) return "system:toolPayload.invalidAge";
  if (needsChoice && !choice) return "system:toolPayload.chooseBackup";
  return null;
}

function toggleEnabled(
  current: ToolPayloadPolicy | null,
  enabled: boolean,
  saved?: ToolPayloadPolicy,
) {
  if (!current) return current;
  const age = !enabled && !validAge(current.age) && saved ? saved.age : current.age;
  return { ...current, enabled, age };
}

export function useToolPayloadRetentionDraft(
  remote: ReturnType<typeof useToolPayloadRetention>,
  admin: boolean,
) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<ToolPayloadPolicy | null>(null);
  const [choice, setChoice] = useState<ToolPayloadBackupChoice | "">("");
  const saved = remote.status?.policy;
  const baseline = useRef<ToolPayloadPolicy | null>(null);
  useEffect(() => {
    if (!saved) return;
    setDraft((current) =>
      !current || serialize(current) === serialize(baseline.current) ? saved : current,
    );
    baseline.current = saved;
  }, [saved]);
  const dirty = Boolean(draft && saved && serialize(draft) !== serialize(saved));
  const needsChoice = needsBackupReview(draft, saved);
  const invalid = !draft || !validAge(draft.age);
  const canEdit = admin && Boolean(remote.status?.supported);
  const validation = validationKey(canEdit, invalid, needsChoice, choice);
  const invalidReason = validation ? t(validation) : undefined;
  useSettingsSaveContributor({
    id: "system:tool-payload-retention",
    order: 26,
    revision: serialize({ draft, choice }),
    isDirty: dirty,
    canSave: canEdit && !remote.pending && !invalid && (!needsChoice || Boolean(choice)),
    invalidReason,
    save: async () => {
      if (!draft || invalid || !canEdit || (needsChoice && !choice)) return;
      const submitted = draft;
      const next = await remote.save({
        ...submitted,
        ...(needsChoice && choice ? { backup_choice: choice } : {}),
      });
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
    setDraft((current) => toggleEnabled(current, enabled, saved));
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
