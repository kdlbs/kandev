"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  getChatDraftAttachments,
  getChatDraftText,
  getSessionStorage,
  removeSessionStorage,
  setChatDraftAttachments,
  setChatDraftContent,
  setChatDraftText,
  setSessionStorage,
  restoreAttachmentPreview,
} from "@/lib/local-storage";
import { deleteAttachment } from "@/lib/api/domains/attachment-api";
import { t } from "@/lib/i18n";
import type { FileAttachment } from "@/components/task/chat/file-attachment";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";

export type QuickChatSetupDraft = {
  message: string;
  attachments: FileAttachment[];
  agentProfileId: string;
  agentProfileExplicit: boolean;
  repositories: TaskRepoRow[];
};

const EMPTY_DRAFT: QuickChatSetupDraft = {
  message: "",
  attachments: [],
  agentProfileId: "",
  agentProfileExplicit: false,
  repositories: [],
};

const QUICK_CHAT_SETUP_PREFERENCES = "kandev.quickChatSetup.preferences";

function quickChatSetupScope(workspaceId: string, userId: string): string {
  return `${userId}\u0000${workspaceId}`;
}

function quickChatSetupDraftId(workspaceId: string, userId: string): string {
  return `quick-chat-setup-draft:${userId}:${workspaceId}`;
}

function preferencesStorageKey(workspaceId: string, userId: string): string {
  return `${QUICK_CHAT_SETUP_PREFERENCES}.${userId}.${workspaceId}`;
}

function restoreSetupAttachments(draftId: string): FileAttachment[] {
  return getChatDraftAttachments(draftId).map((stored) => {
    const restored = restoreAttachmentPreview(stored);
    const hasRecoverableData = Boolean(restored.attachmentId || restored.data);
    return {
      ...restored,
      uploadStatus: hasRecoverableData ? "ready" : "failed",
      ...(!hasRecoverableData ? { uploadError: t("task:attachmentUploadFailed") } : {}),
    };
  });
}

function readQuickChatSetupDraft(workspaceId: string, userId: string): QuickChatSetupDraft {
  const draftId = quickChatSetupDraftId(workspaceId, userId);
  const preferences = getSessionStorage(preferencesStorageKey(workspaceId, userId), {
    agentProfileId: "",
    agentProfileExplicit: false,
    repositories: [] as Array<{
      key: string;
      repositoryId: string | null;
      branch: string;
      baseBranch: string | null;
    }>,
  });
  return {
    message: getChatDraftText(draftId),
    attachments: restoreSetupAttachments(draftId),
    agentProfileId: preferences.agentProfileId,
    agentProfileExplicit: preferences.agentProfileExplicit,
    repositories: preferences.repositories.map((row) => ({
      key: row.key,
      repositoryId: row.repositoryId ?? undefined,
      branch: row.branch,
      baseBranch: row.baseBranch ?? undefined,
    })),
  };
}

function persistQuickChatSetupDraft(
  workspaceId: string,
  userId: string,
  draft: QuickChatSetupDraft,
): void {
  const draftId = quickChatSetupDraftId(workspaceId, userId);
  setChatDraftText(draftId, draft.message);
  setChatDraftAttachments(draftId, draft.attachments);
  setSessionStorage(preferencesStorageKey(workspaceId, userId), {
    agentProfileId: draft.agentProfileId,
    agentProfileExplicit: draft.agentProfileExplicit,
    repositories: draft.repositories.map((row) => ({
      key: row.key,
      repositoryId: row.repositoryId ?? null,
      branch: row.branch,
      baseBranch: row.baseBranch ?? null,
    })),
  });
}

function clearStoredQuickChatSetupDraft(workspaceId: string, userId: string): void {
  const draftId = quickChatSetupDraftId(workspaceId, userId);
  setChatDraftText(draftId, "");
  setChatDraftContent(draftId, null);
  setChatDraftAttachments(draftId, []);
  removeSessionStorage(preferencesStorageKey(workspaceId, userId));
}

export function useQuickChatSetupDraft(workspaceId: string, userId = "local") {
  const scope = quickChatSetupScope(workspaceId, userId);
  const [draftsByScope, setDraftsByScope] = useState(() => ({
    [scope]: readQuickChatSetupDraft(workspaceId, userId),
  }));
  const latestDrafts = useRef(draftsByScope);
  const generations = useRef<Record<string, number>>({});
  const generation = generations.current[scope] ?? 0;
  const fallbackDraft = useMemo(
    () => readQuickChatSetupDraft(workspaceId, userId),
    [scope, userId, workspaceId],
  );
  const draft = draftsByScope[scope] ?? fallbackDraft;

  useEffect(() => {
    const storedDraft = draftsByScope[scope];
    if (storedDraft) persistQuickChatSetupDraft(workspaceId, userId, storedDraft);
    latestDrafts.current = draftsByScope;
  }, [draftsByScope, scope, userId, workspaceId]);

  const update = useCallback(
    (patch: Partial<QuickChatSetupDraft>): boolean => {
      if ((generations.current[scope] ?? 0) !== generation) return false;
      const current = latestDrafts.current[scope] ?? fallbackDraft;
      const next = { ...current, ...patch };
      const updated = { ...latestDrafts.current, [scope]: next };
      latestDrafts.current = updated;
      setDraftsByScope(updated);
      return true;
    },
    [fallbackDraft, generation, scope],
  );

  const clear = useCallback(
    (discardAttachments: boolean) => {
      const current = latestDrafts.current[scope] ?? fallbackDraft;
      generations.current[scope] = (generations.current[scope] ?? 0) + 1;
      clearStoredQuickChatSetupDraft(workspaceId, userId);
      latestDrafts.current = { ...latestDrafts.current, [scope]: EMPTY_DRAFT };
      setDraftsByScope(latestDrafts.current);
      if (discardAttachments) {
        current.attachments.forEach((attachment) => {
          if (attachment.attachmentId)
            void deleteAttachment(attachment.attachmentId).catch(() => undefined);
        });
      }
    },
    [fallbackDraft, scope, userId, workspaceId],
  );

  return { draft, update, clear };
}
