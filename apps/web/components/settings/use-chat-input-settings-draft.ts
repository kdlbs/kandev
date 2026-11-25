"use client";

import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { updateUserSettings } from "@/lib/api";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import { mapLatestUserSettingsResponse } from "@/lib/ssr/user-settings";

type ChatInputSettingsDraft = Pick<UserSettingsState, "chatSubmitKey" | "escapeCancelsAgent">;

function readChatInputSettingsDraft(settings: UserSettingsState): ChatInputSettingsDraft {
  return {
    chatSubmitKey: settings.chatSubmitKey,
    escapeCancelsAgent: settings.escapeCancelsAgent,
  };
}

function rebaseChatInputSettingsDraft(
  draft: ChatInputSettingsDraft,
  baseline: ChatInputSettingsDraft,
  nextSaved: ChatInputSettingsDraft,
  preserveDraftFields: ReadonlySet<keyof ChatInputSettingsDraft> = new Set(),
): ChatInputSettingsDraft {
  return {
    chatSubmitKey:
      preserveDraftFields.has("chatSubmitKey") || draft.chatSubmitKey !== baseline.chatSubmitKey
        ? draft.chatSubmitKey
        : nextSaved.chatSubmitKey,
    escapeCancelsAgent:
      preserveDraftFields.has("escapeCancelsAgent") ||
      draft.escapeCancelsAgent !== baseline.escapeCancelsAgent
        ? draft.escapeCancelsAgent
        : nextSaved.escapeCancelsAgent,
  };
}

export function useChatInputSettingsDraft() {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSavedState] = useState(() => readChatInputSettingsDraft(userSettings));
  const [draft, setDraftState] = useState(saved);
  const savedRef = useRef(saved);
  const draftRef = useRef(draft);
  const editedDuringSaveRef = useRef<Set<keyof ChatInputSettingsDraft> | null>(null);
  savedRef.current = saved;
  draftRef.current = draft;

  const replaceSnapshots = useCallback(
    (nextSaved: ChatInputSettingsDraft, nextDraft: ChatInputSettingsDraft) => {
      savedRef.current = nextSaved;
      draftRef.current = nextDraft;
      setSavedState(nextSaved);
      setDraftState(nextDraft);
    },
    [],
  );
  const updateDraft = useCallback((patch: Partial<ChatInputSettingsDraft>) => {
    const current = draftRef.current;
    const next = { ...current, ...patch };
    for (const field of Object.keys(patch) as Array<keyof ChatInputSettingsDraft>) {
      if (current[field] !== next[field]) editedDuringSaveRef.current?.add(field);
    }
    draftRef.current = next;
    setDraftState(next);
  }, []);

  useLayoutEffect(() => {
    const nextSaved = readChatInputSettingsDraft(userSettings);
    const nextDraft = rebaseChatInputSettingsDraft(
      draftRef.current,
      savedRef.current,
      nextSaved,
      editedDuringSaveRef.current ?? undefined,
    );
    replaceSnapshots(nextSaved, nextDraft);
  }, [replaceSnapshots, userSettings]);

  const revision = JSON.stringify(draft);
  useSettingsSaveContributor({
    id: "general-chat-submit-key",
    revision,
    isDirty: userSettings.loaded && revision !== JSON.stringify(saved),
    canSave: userSettings.loaded,
    save: async () => {
      const submitted = draftRef.current;
      const baseline = savedRef.current;
      const current = storeApi.getState().userSettings;
      const patch = {
        workspace_id: current.workspaceId || "",
        repository_ids: current.repositoryIds || [],
        ...(submitted.chatSubmitKey !== baseline.chatSubmitKey
          ? { chat_submit_key: submitted.chatSubmitKey }
          : {}),
        ...(submitted.escapeCancelsAgent !== baseline.escapeCancelsAgent
          ? { escape_cancels_agent: submitted.escapeCancelsAgent }
          : {}),
      };
      const editedDuringSave = new Set<keyof ChatInputSettingsDraft>();
      editedDuringSaveRef.current = editedDuringSave;
      try {
        const response = await updateUserSettings(patch);
        const latest = storeApi.getState().userSettings;
        const authoritative = mapLatestUserSettingsResponse(response, latest);
        const nextSaved = readChatInputSettingsDraft(authoritative);
        const nextDraft = rebaseChatInputSettingsDraft(
          draftRef.current,
          submitted,
          nextSaved,
          editedDuringSave,
        );
        replaceSnapshots(nextSaved, nextDraft);
        if (authoritative !== latest) setUserSettings(authoritative);
      } finally {
        if (editedDuringSaveRef.current === editedDuringSave) {
          editedDuringSaveRef.current = null;
        }
      }
    },
    discard: () => replaceSnapshots(savedRef.current, savedRef.current),
  });

  return { saved, draft, updateDraft };
}
