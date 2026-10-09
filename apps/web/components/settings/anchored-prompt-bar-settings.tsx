"use client";
import { SettingsInfo } from "./settings-info";

import { useEffect, useRef, useState } from "react";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { SettingsCard } from "./settings-card";
import { SettingsRow, type SettingsPresentation } from "./settings-group";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { useTranslation } from "react-i18next";

type TranscriptNavigationSettings = {
  showAnchoredPromptBar: boolean;
  showScrollToLastPrompt: boolean;
  showScrollToStart: boolean;
  showTranscriptAutoScrollControl: boolean;
};

function sameSettings(
  left: TranscriptNavigationSettings,
  right: TranscriptNavigationSettings,
): boolean {
  return (
    left.showAnchoredPromptBar === right.showAnchoredPromptBar &&
    left.showScrollToLastPrompt === right.showScrollToLastPrompt &&
    left.showScrollToStart === right.showScrollToStart &&
    left.showTranscriptAutoScrollControl === right.showTranscriptAutoScrollControl
  );
}

function changedSettings(
  saved: TranscriptNavigationSettings,
  draft: TranscriptNavigationSettings,
): Record<string, boolean> {
  const changes: Record<string, boolean> = {};
  if (saved.showAnchoredPromptBar !== draft.showAnchoredPromptBar) {
    changes.show_anchored_prompt_bar = draft.showAnchoredPromptBar;
  }
  if (saved.showScrollToLastPrompt !== draft.showScrollToLastPrompt) {
    changes.show_scroll_to_last_prompt = draft.showScrollToLastPrompt;
  }
  if (saved.showScrollToStart !== draft.showScrollToStart) {
    changes.show_scroll_to_start = draft.showScrollToStart;
  }
  if (saved.showTranscriptAutoScrollControl !== draft.showTranscriptAutoScrollControl) {
    changes.show_transcript_auto_scroll_control = draft.showTranscriptAutoScrollControl;
  }
  return changes;
}

type TranscriptNavigationSwitchProps = {
  id: string;
  label: string;
  description: string;
  info: string;
  checked: boolean;
  isDirty: boolean;
  onCheckedChange: (checked: boolean) => void;
  presentation?: SettingsPresentation;
  discoveryTargetId?: string;
};

function TranscriptNavigationSwitch({
  id,
  label,
  description,
  info,
  checked,
  isDirty,
  onCheckedChange,
  presentation = "card",
  discoveryTargetId,
}: TranscriptNavigationSwitchProps) {
  const control = (
    <Switch
      id={id}
      checked={checked}
      data-settings-dirty={isDirty}
      onCheckedChange={onCheckedChange}
      className="shrink-0 cursor-pointer"
    />
  );

  if (presentation === "row") {
    return (
      <SettingsRow
        label={label}
        description={description}
        info={<SettingsInfo label={label}>{info}</SettingsInfo>}
        controlId={id}
        touchTarget="switch"
        discoveryTargetId={discoveryTargetId}
        isDirty={isDirty}
        control={control}
      />
    );
  }

  return (
    <div className="flex min-h-11 items-center justify-between gap-4">
      <div className="min-w-0 space-y-0.5">
        <Label htmlFor={id}>{label}</Label>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      {control}
    </div>
  );
}

function TranscriptNavigationSwitches({
  draft,
  isDirty,
  presentation,
  onChange,
}: {
  draft: TranscriptNavigationSettings;
  isDirty: boolean;
  presentation: SettingsPresentation;
  onChange: (update: Partial<TranscriptNavigationSettings>) => void;
}) {
  const { t } = useTranslation();
  const discoveryTargetId =
    presentation === "row" ? GENERAL_SETTINGS_TARGETS.transcriptNavigation : undefined;

  return (
    <>
      <TranscriptNavigationSwitch
        id="show-anchored-prompt-bar"
        label={t("settings:showAnchoredPromptBar")}
        description={t("settings:anchoredPromptShort")}
        info={t("settings:desktopOnlyWhileYouScrollPast")}
        checked={draft.showAnchoredPromptBar}
        isDirty={isDirty}
        presentation={presentation}
        discoveryTargetId={discoveryTargetId}
        onCheckedChange={(showAnchoredPromptBar) => onChange({ showAnchoredPromptBar })}
      />
      <TranscriptNavigationSwitch
        id="show-scroll-to-last-prompt"
        label={t("settings:showScrollToLastPrompt")}
        description={t("settings:jumpPromptShort")}
        info={t("settings:showTheJumpControlAfterYour")}
        checked={draft.showScrollToLastPrompt}
        isDirty={isDirty}
        presentation={presentation}
        onCheckedChange={(showScrollToLastPrompt) => onChange({ showScrollToLastPrompt })}
      />
      <TranscriptNavigationSwitch
        id="show-scroll-to-start"
        label={t("settings:showScrollToStart")}
        description={t("settings:jumpStartShort")}
        info={t("settings:showTheControlThatJumpsTo")}
        checked={draft.showScrollToStart}
        isDirty={isDirty}
        presentation={presentation}
        onCheckedChange={(showScrollToStart) => onChange({ showScrollToStart })}
      />
      <TranscriptNavigationSwitch
        id="show-transcript-auto-scroll-control"
        label={t("settings:showTranscriptAutoScrollControl")}
        description={t("settings:autoScrollShort")}
        info={t("settings:showThePerSessionButtonThat")}
        checked={draft.showTranscriptAutoScrollControl}
        isDirty={isDirty}
        presentation={presentation}
        onCheckedChange={(showTranscriptAutoScrollControl) =>
          onChange({ showTranscriptAutoScrollControl })
        }
      />
    </>
  );
}

export function AnchoredPromptBarSettings({
  presentation = "card",
}: {
  presentation?: SettingsPresentation;
}) {
  const { t } = useTranslation();
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const current = {
    showAnchoredPromptBar: userSettings.showAnchoredPromptBar,
    showScrollToLastPrompt: userSettings.showScrollToLastPrompt,
    showScrollToStart: userSettings.showScrollToStart,
    showTranscriptAutoScrollControl: userSettings.showTranscriptAutoScrollControl,
  };
  const [saved, setSaved] = useState(current);
  const [draft, setDraft] = useState(current);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = !sameSettings(draft, saved);

  useEffect(() => {
    setSaved((previous) => {
      if (sameSettings(draftRef.current, previous)) setDraft(current);
      return current;
    });
  }, [
    current.showAnchoredPromptBar,
    current.showScrollToLastPrompt,
    current.showScrollToStart,
    current.showTranscriptAutoScrollControl,
  ]);

  useSettingsSaveContributor({
    id: "general-transcript-navigation",
    revision: JSON.stringify(draft),
    isDirty,
    save: async (revision) => {
      const submitted = JSON.parse(String(revision)) as TranscriptNavigationSettings;
      const changes = changedSettings(saved, submitted);
      await updateUserSettings(changes);
      setSaved(submitted);
      setUserSettings({ ...storeApi.getState().userSettings, ...submitted });
    },
    discard: () => setDraft(saved),
  });

  const switches = (
    <TranscriptNavigationSwitches
      draft={draft}
      isDirty={isDirty}
      presentation={presentation}
      onChange={(update) => setDraft((previous) => ({ ...previous, ...update }))}
    />
  );

  if (presentation === "row") {
    return (
      <div
        data-testid="anchored-prompt-bar-card"
        data-settings-dirty={isDirty}
        className="divide-y divide-border/70"
      >
        {switches}
      </div>
    );
  }

  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.transcriptNavigation}
      data-testid="anchored-prompt-bar-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:transcriptNavigation")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">{switches}</CardContent>
    </SettingsCard>
  );
}
