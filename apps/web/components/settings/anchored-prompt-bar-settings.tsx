"use client";

import { useEffect, useRef, useState } from "react";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { SettingsCard } from "./settings-card";
import { useSettingsSaveContributor } from "./settings-save-provider";

type TranscriptNavigationSettings = {
  showAnchoredPromptBar: boolean;
  showScrollToLastPrompt: boolean;
  showScrollToStart: boolean;
};

function sameSettings(
  left: TranscriptNavigationSettings,
  right: TranscriptNavigationSettings,
): boolean {
  return (
    left.showAnchoredPromptBar === right.showAnchoredPromptBar &&
    left.showScrollToLastPrompt === right.showScrollToLastPrompt &&
    left.showScrollToStart === right.showScrollToStart
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
  return changes;
}

export function AnchoredPromptBarSettings() {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const current = {
    showAnchoredPromptBar: userSettings.showAnchoredPromptBar,
    showScrollToLastPrompt: userSettings.showScrollToLastPrompt,
    showScrollToStart: userSettings.showScrollToStart,
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
  }, [current.showAnchoredPromptBar, current.showScrollToLastPrompt, current.showScrollToStart]);

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

  return (
    <SettingsCard isDirty={isDirty} data-testid="anchored-prompt-bar-card">
      <CardHeader>
        <CardTitle className="text-base">Transcript Navigation</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex min-h-11 items-center justify-between gap-4">
          <div className="min-w-0 space-y-0.5">
            <Label htmlFor="show-anchored-prompt-bar">Show anchored prompt bar</Label>
            <p className="text-xs text-muted-foreground">
              Desktop only. While you scroll past your last prompt, stick a shortened copy to the
              top of the transcript with an expand toggle.
            </p>
          </div>
          <Switch
            id="show-anchored-prompt-bar"
            checked={draft.showAnchoredPromptBar}
            data-settings-dirty={isDirty}
            onCheckedChange={(showAnchoredPromptBar) =>
              setDraft((previous) => ({ ...previous, showAnchoredPromptBar }))
            }
            className="shrink-0 cursor-pointer"
          />
        </div>
        <div className="flex min-h-11 items-center justify-between gap-4">
          <div className="min-w-0 space-y-0.5">
            <Label htmlFor="show-scroll-to-last-prompt">Show scroll to last prompt</Label>
            <p className="text-xs text-muted-foreground">
              Show the jump control after your latest prompt scrolls above the transcript.
            </p>
          </div>
          <Switch
            id="show-scroll-to-last-prompt"
            checked={draft.showScrollToLastPrompt}
            data-settings-dirty={isDirty}
            onCheckedChange={(showScrollToLastPrompt) =>
              setDraft((previous) => ({ ...previous, showScrollToLastPrompt }))
            }
            className="shrink-0 cursor-pointer"
          />
        </div>
        <div className="flex min-h-11 items-center justify-between gap-4">
          <div className="min-w-0 space-y-0.5">
            <Label htmlFor="show-scroll-to-start">Show scroll to start</Label>
            <p className="text-xs text-muted-foreground">
              Show the control that jumps to the beginning of the transcript.
            </p>
          </div>
          <Switch
            id="show-scroll-to-start"
            checked={draft.showScrollToStart}
            data-settings-dirty={isDirty}
            onCheckedChange={(showScrollToStart) =>
              setDraft((previous) => ({ ...previous, showScrollToStart }))
            }
            className="shrink-0 cursor-pointer"
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}
