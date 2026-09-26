"use client";

import { useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import type { UpdatesChannel, UpdatesResponse } from "@/lib/types/system";
import { useSettingsSaveContributor } from "../settings-save-provider";
import { updateChannelUnsupportedReasonKey } from "./update-channel-reasons";

export function useUpdateChannelDraft(
  updates: UpdatesResponse | null | undefined,
  saveChannel: (channel: UpdatesChannel) => Promise<UpdatesResponse>,
) {
  const authoritative = updates?.channel ?? "stable";
  const editable = updates?.channel_editable === true;
  const [saved, setSaved] = useState<UpdatesChannel>(authoritative);
  const savedRef = useRef(saved);
  const [draft, setDraftState] = useState<UpdatesChannel>(authoritative);
  const draftRef = useRef(draft);
  const [isSaving, setIsSaving] = useState(false);
  // A newer edit can circle back to the old saved value, so value equality
  // alone cannot decide whether an in-flight save response may rebase it.
  const draftRevisionRef = useRef(0);
  const pendingSaveRevisionRef = useRef<number | null>(null);
  draftRef.current = draft;
  const isDirty = editable && draft !== saved;

  const setDraft = (channel: UpdatesChannel) => {
    draftRevisionRef.current += 1;
    draftRef.current = channel;
    setDraftState(channel);
  };

  // The settings coordinator snapshots this contributor before it begins
  // saving earlier contributors. Capture the edit token in that snapshot; the
  // matching save callback closes over this render's draft value.
  const contributorRevision = draftRevisionRef.current;

  useLayoutEffect(() => {
    const previous = savedRef.current;
    savedRef.current = authoritative;
    setSaved(authoritative);
    const pendingRevision = pendingSaveRevisionRef.current;
    const hasNewerDraft = pendingRevision !== null && draftRevisionRef.current !== pendingRevision;
    if (!editable || (draftRef.current === previous && !hasNewerDraft)) {
      draftRef.current = authoritative;
      setDraftState(authoritative);
    }
  }, [authoritative, editable]);

  useSettingsSaveContributor({
    id: "system-updates-channel",
    order: 10,
    revision: contributorRevision,
    // Keep the navigation guard active for the full save. A user can edit the
    // draft back to the previously saved value while the submitted channel is
    // still in flight, but leaving then would abandon an unresolved mutation.
    isDirty: isDirty || isSaving,
    save: async (revision) => {
      const submitted = draft;
      const submittedRevision = revision as number;
      pendingSaveRevisionRef.current = submittedRevision;
      setIsSaving(true);
      try {
        const response = await saveChannel(submitted);
        savedRef.current = response.channel;
        setSaved(response.channel);
        if (draftRevisionRef.current === submittedRevision) {
          draftRef.current = response.channel;
          setDraftState(response.channel);
        }
      } finally {
        if (pendingSaveRevisionRef.current === submittedRevision) {
          pendingSaveRevisionRef.current = null;
        }
        setIsSaving(false);
      }
    },
    discard: () => setDraft(savedRef.current),
  });

  return {
    draft,
    editable,
    isDirty,
    isSaving,
    unsupportedReason: updates?.channel_unsupported_reason ?? "",
    setDraft,
  };
}

type UpdateChannelControlProps = ReturnType<typeof useUpdateChannelDraft>;

export function UpdateChannelControl({
  draft,
  editable,
  isDirty,
  unsupportedReason,
  setDraft,
}: UpdateChannelControlProps) {
  const { t } = useTranslation();
  const reasonId = "system-updates-channel-reason";
  const reason = unsupportedReason ? t(updateChannelUnsupportedReasonKey(unsupportedReason)) : "";
  return (
    <div
      className="min-w-0 space-y-2"
      data-testid="system-updates-channel"
      data-settings-dirty={isDirty}
    >
      <div>
        <div className="text-sm font-medium">{t("settings:updateChannel")}</div>
        <p className="text-xs text-muted-foreground">{t("settings:updateChannelDescription")}</p>
      </div>
      <RadioGroup
        aria-label={t("settings:updateChannel")}
        value={draft}
        onValueChange={(value) => {
          if (editable && (value === "stable" || value === "nightly")) setDraft(value);
        }}
        className="gap-2"
        data-settings-dirty={isDirty}
      >
        <UpdateChannelOption
          channel="stable"
          label={t("settings:updateChannelStable")}
          description={t("settings:updateChannelStableDescription")}
          disabled={!editable}
          reasonId={!editable && reason ? reasonId : undefined}
        />
        <UpdateChannelOption
          channel="nightly"
          label={t("settings:updateChannelNightly")}
          description={t("settings:updateChannelNightlyDescription")}
          disabled={!editable}
          reasonId={!editable && reason ? reasonId : undefined}
        />
      </RadioGroup>
      {!editable && reason && (
        <p
          id={reasonId}
          className="break-words text-xs text-muted-foreground"
          data-testid="system-updates-channel-reason"
        >
          {reason}
        </p>
      )}
    </div>
  );
}

function UpdateChannelOption({
  channel,
  label,
  description,
  disabled,
  reasonId,
}: {
  channel: UpdatesChannel;
  label: string;
  description: string;
  disabled: boolean;
  reasonId?: string;
}) {
  const inputId = `system-updates-channel-${channel}-input`;
  const labelId = `system-updates-channel-${channel}-label`;
  const descriptionId = `system-updates-channel-${channel}-description`;
  const describedBy = reasonId ? `${descriptionId} ${reasonId}` : descriptionId;
  return (
    <Label
      htmlFor={inputId}
      className={`flex min-h-11 w-full min-w-0 items-start gap-3 rounded-md border p-3 ${
        disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer hover:bg-muted/30"
      }`}
      data-testid={`system-updates-channel-${channel}`}
    >
      <RadioGroupItem
        id={inputId}
        value={channel}
        disabled={disabled}
        aria-labelledby={labelId}
        aria-describedby={describedBy}
        className="mt-0.5"
      />
      <span className="min-w-0 space-y-1">
        <span id={labelId} className="block text-sm font-medium">
          {label}
        </span>
        <span
          id={descriptionId}
          className="block whitespace-normal break-words text-xs text-muted-foreground"
        >
          {description}
        </span>
      </span>
    </Label>
  );
}
