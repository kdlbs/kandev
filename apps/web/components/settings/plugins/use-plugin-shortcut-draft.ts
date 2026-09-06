"use client";

import {
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import type { StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";
import { compareUserSettingsRevisions } from "@/lib/settings/user-settings-revision";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import type { UserSettingsResponse } from "@/lib/types/http";
import { useSettingsSaveContributor } from "../settings-save-provider";

type ShortcutOverride = StoredShortcutOverrides[string];

function modifierMapsEqual(
  left: ShortcutOverride["modifiers"],
  right: ShortcutOverride["modifiers"],
): boolean {
  const leftEntries = Object.entries(left ?? {});
  const rightEntries = Object.entries(right ?? {});
  return (
    leftEntries.length === rightEntries.length &&
    leftEntries.every(([modifier, enabled]) => right?.[modifier] === enabled)
  );
}

function shortcutOverridesEqual(
  left: ShortcutOverride | undefined,
  right: ShortcutOverride | undefined,
): boolean {
  if (!left || !right) return left === right;
  return left.key === right.key && modifierMapsEqual(left.modifiers, right.modifiers);
}

function overrideMapsEqual(left: StoredShortcutOverrides, right: StoredShortcutOverrides): boolean {
  const leftIds = Object.keys(left);
  return (
    leftIds.length === Object.keys(right).length &&
    leftIds.every((id) => shortcutOverridesEqual(left[id], right[id]))
  );
}

export function rebaseShortcutOverrides(
  draft: StoredShortcutOverrides,
  baseline: StoredShortcutOverrides,
  nextBaseline: StoredShortcutOverrides,
): StoredShortcutOverrides {
  const rebased = { ...nextBaseline };
  const candidateIds = new Set([...Object.keys(baseline), ...Object.keys(draft)]);
  for (const id of candidateIds) {
    const draftOverride = draft[id];
    if (shortcutOverridesEqual(draftOverride, baseline[id])) continue;
    if (draftOverride) rebased[id] = draftOverride;
    else delete rebased[id];
  }
  return rebased;
}

function shortcutOverridesRevision(overrides: StoredShortcutOverrides): string {
  return JSON.stringify(
    Object.keys(overrides)
      .sort()
      .map((id) => {
        const override = overrides[id];
        return [id, override.key, Object.entries(override.modifiers ?? {}).sort()];
      }),
  );
}

function selectAuthoritativeSettings(
  response: UserSettingsResponse,
  settingsAtSubmit: UserSettingsState,
  latestSettings: UserSettingsState,
): { settings: UserSettingsState; responseIsCurrent: boolean } {
  const responseOrder = compareUserSettingsRevisions(
    response.settings.revision,
    latestSettings.revision,
  );
  const responseIsCurrent =
    responseOrder === null ? latestSettings === settingsAtSubmit : responseOrder >= 0;
  return {
    settings: responseIsCurrent
      ? mapUserSettingsResponse(response, latestSettings)
      : latestSettings,
    responseIsCurrent,
  };
}

export function usePluginShortcutDraft(contributorId: string): {
  saved: StoredShortcutOverrides;
  draft: StoredShortcutOverrides;
  setDraft: Dispatch<SetStateAction<StoredShortcutOverrides>>;
  isDirty: boolean;
} {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const initial = { ...userSettings.keyboardShortcuts } as StoredShortcutOverrides;
  const [saved, setSavedState] = useState(initial);
  const [draft, setDraftState] = useState(initial);
  const savedRef = useRef(saved);
  const draftRef = useRef(draft);
  const synchronizedSettingsRef = useRef(userSettings);
  savedRef.current = saved;
  draftRef.current = draft;

  const replaceSnapshots = useCallback(
    (nextSaved: StoredShortcutOverrides, nextDraft: StoredShortcutOverrides) => {
      savedRef.current = nextSaved;
      draftRef.current = nextDraft;
      setSavedState(nextSaved);
      setDraftState(nextDraft);
    },
    [],
  );
  const setDraft = useCallback<Dispatch<SetStateAction<StoredShortcutOverrides>>>((update) => {
    const next =
      typeof update === "function"
        ? (update as (current: StoredShortcutOverrides) => StoredShortcutOverrides)(
            draftRef.current,
          )
        : update;
    draftRef.current = next;
    setDraftState(next);
  }, []);

  useLayoutEffect(() => {
    if (synchronizedSettingsRef.current === userSettings) return;
    const nextSaved = { ...userSettings.keyboardShortcuts } as StoredShortcutOverrides;
    const nextDraft = rebaseShortcutOverrides(draftRef.current, savedRef.current, nextSaved);
    synchronizedSettingsRef.current = userSettings;
    replaceSnapshots(nextSaved, nextDraft);
  }, [replaceSnapshots, userSettings]);

  const revision = shortcutOverridesRevision(draft);
  const synchronized = synchronizedSettingsRef.current === userSettings;
  const isDirty = userSettings.loaded && synchronized && !overrideMapsEqual(draft, saved);

  useSettingsSaveContributor({
    id: contributorId,
    revision,
    isDirty,
    canSave: userSettings.loaded && synchronized,
    save: async (submittedRevision) => {
      const submittedDraft = draft;
      const settingsAtSubmit = storeApi.getState().userSettings;
      const submitted = rebaseShortcutOverrides(
        submittedDraft,
        saved,
        settingsAtSubmit.keyboardShortcuts as StoredShortcutOverrides,
      );
      const response = await updateUserSettings({ keyboard_shortcuts: submitted });
      const latestSettings = storeApi.getState().userSettings;
      const authoritative = selectAuthoritativeSettings(response, settingsAtSubmit, latestSettings);
      const nextSaved = {
        ...authoritative.settings.keyboardShortcuts,
      } as StoredShortcutOverrides;
      const draftChanged = shortcutOverridesRevision(draftRef.current) !== submittedRevision;
      const nextDraft = draftChanged
        ? rebaseShortcutOverrides(draftRef.current, submittedDraft, nextSaved)
        : nextSaved;
      replaceSnapshots(nextSaved, nextDraft);
      if (authoritative.responseIsCurrent) setUserSettings(authoritative.settings);
    },
    discard: () => setDraft(savedRef.current),
  });

  return { saved, draft, setDraft, isDirty };
}
