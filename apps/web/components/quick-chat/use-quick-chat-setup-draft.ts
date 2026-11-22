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
import type { QuickChatOpeningPayload } from "@/lib/state/slices/ui/types";
import type { QuickChatRepositoryInput } from "@/lib/api/domains/workspace-api";
import {
  QUICK_CHAT_RUNTIME_ID,
  type PendingQuickChatCreation,
  toQuickChatAttachmentDescriptors,
} from "./quick-chat-creation";
import { generateUUID } from "@/lib/utils";

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

function creationStorageKey(workspaceId: string, userId: string): string {
  return `quick-chat-creation:${userId}:${workspaceId}`;
}

function readPendingCreation(workspaceId: string, userId: string): PendingQuickChatCreation | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.sessionStorage.getItem(creationStorageKey(workspaceId, userId));
    if (!raw) return null;
    const value = JSON.parse(raw) as Partial<PendingQuickChatCreation>;
    if (
      typeof value.creationId !== "string" ||
      typeof value.clientMessageId !== "string" ||
      typeof value.message !== "string" ||
      typeof value.agentProfileId !== "string" ||
      !Array.isArray(value.repositories) ||
      !Array.isArray(value.attachments)
    )
      return null;
    return {
      ...value,
      state: value.state === "submitting" ? "unknown" : value.state,
    } as PendingQuickChatCreation;
  } catch {
    return null;
  }
}

function pendingCreationForScope(
  pendingByScope: Record<string, PendingQuickChatCreation | null>,
  scope: string,
  workspaceId: string,
  userId: string,
): PendingQuickChatCreation | null {
  return Object.prototype.hasOwnProperty.call(pendingByScope, scope)
    ? (pendingByScope[scope] ?? null)
    : readPendingCreation(workspaceId, userId);
}

function writePendingCreation(
  workspaceId: string,
  userId: string,
  pending: PendingQuickChatCreation | null,
): void {
  if (typeof window === "undefined") return;
  try {
    const key = creationStorageKey(workspaceId, userId);
    if (!pending) window.sessionStorage.removeItem(key);
    else window.sessionStorage.setItem(key, JSON.stringify(pending));
  } catch {
    // Keep browser storage failure from blocking Quick Chat creation.
  }
}

function restoreSetupAttachments(draftId: string): FileAttachment[] {
  return getChatDraftAttachments(draftId).map((stored) => {
    const restored = restoreAttachmentPreview(stored);
    const expiresAt = restored.expiresAt ? Date.parse(restored.expiresAt) : Number.NaN;
    const isExpired = Number.isFinite(expiresAt) && expiresAt <= Date.now();
    const hasRecoverableData = Boolean(restored.data || (restored.attachmentId && !isExpired));
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

function useQuickChatDraftState(workspaceId: string, userId: string, scope: string) {
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

  const clearCapturedDraft = useCallback(
    (
      message: string,
      attachments: Array<Pick<FileAttachment, "id" | "attachmentId" | "fileName" | "mimeType">>,
    ) => {
      const current = latestDrafts.current[scope] ?? fallbackDraft;
      const sameAttachments =
        current.attachments.length === attachments.length &&
        current.attachments.every((item, index) => {
          const captured = attachments[index];
          const sameUpload =
            item.attachmentId && captured?.attachmentId
              ? item.attachmentId === captured.attachmentId
              : item.id === captured?.id;
          return (
            sameUpload &&
            item.fileName === captured?.fileName &&
            item.mimeType === captured?.mimeType
          );
        });
      if (current.message !== message || !sameAttachments) return false;
      const next = { ...current, message: "", attachments: [] };
      const updated = { ...latestDrafts.current, [scope]: next };
      latestDrafts.current = updated;
      setDraftsByScope(updated);
      setChatDraftText(quickChatSetupDraftId(workspaceId, userId), "");
      setChatDraftContent(quickChatSetupDraftId(workspaceId, userId), null);
      setChatDraftAttachments(quickChatSetupDraftId(workspaceId, userId), []);
      return true;
    },
    [fallbackDraft, scope, userId, workspaceId],
  );

  return {
    draft,
    update,
    clear,
    clearCapturedDraft,
  };
}

function usePendingQuickChatCreation(workspaceId: string, userId: string, scope: string) {
  const [pendingByScope, setPendingByScope] = useState(() => ({
    [scope]: readPendingCreation(workspaceId, userId),
  }));
  const latestPending = useRef(pendingByScope);
  const pendingCreation = pendingCreationForScope(pendingByScope, scope, workspaceId, userId);

  useEffect(() => {
    if (!Object.prototype.hasOwnProperty.call(pendingByScope, scope)) {
      const hydrated = { ...pendingByScope, [scope]: readPendingCreation(workspaceId, userId) };
      latestPending.current = hydrated;
      setPendingByScope(hydrated);
      return;
    }
    latestPending.current = pendingByScope;
    writePendingCreation(workspaceId, userId, pendingByScope[scope] ?? null);
  }, [pendingByScope, scope, userId, workspaceId]);

  const beginPendingCreation = useCallback(
    (
      payload: QuickChatOpeningPayload,
      agentProfileId: string,
      repositories: QuickChatRepositoryInput[],
    ): PendingQuickChatCreation | null => {
      if (pendingCreationForScope(latestPending.current, scope, workspaceId, userId)) return null;
      const pending: PendingQuickChatCreation = {
        creationId: generateUUID(),
        clientMessageId: payload.clientMessageId ?? generateUUID(),
        message: payload.message,
        attachments: toQuickChatAttachmentDescriptors(payload.attachments),
        agentProfileId,
        repositories: repositories.map((row) => ({ ...row })),
        runtimeId: QUICK_CHAT_RUNTIME_ID,
        state: "submitting",
        messageAttempted: false,
      };
      const updated = { ...latestPending.current, [scope]: pending };
      latestPending.current = updated;
      setPendingByScope(updated);
      writePendingCreation(workspaceId, userId, pending);
      return pending;
    },
    [scope, userId, workspaceId],
  );

  const updatePendingCreation = useCallback(
    (creationId: string, patch: Partial<PendingQuickChatCreation>) => {
      const current = pendingCreationForScope(latestPending.current, scope, workspaceId, userId);
      if (!current || current.creationId !== creationId) return false;
      const next = { ...current, ...patch };
      const updated = { ...latestPending.current, [scope]: next };
      latestPending.current = updated;
      setPendingByScope(updated);
      writePendingCreation(workspaceId, userId, next);
      return true;
    },
    [scope, userId, workspaceId],
  );

  const clearPendingCreation = useCallback(
    (creationId?: string) => {
      const current = pendingCreationForScope(latestPending.current, scope, workspaceId, userId);
      if (!current || (creationId && current.creationId !== creationId)) return false;
      const updated = { ...latestPending.current, [scope]: null };
      latestPending.current = updated;
      setPendingByScope(updated);
      writePendingCreation(workspaceId, userId, null);
      return true;
    },
    [scope, userId, workspaceId],
  );

  return { pendingCreation, beginPendingCreation, updatePendingCreation, clearPendingCreation };
}

export function useQuickChatSetupDraft(workspaceId: string, userId = "local") {
  const scope = quickChatSetupScope(workspaceId, userId);
  const draft = useQuickChatDraftState(workspaceId, userId, scope);
  const creation = usePendingQuickChatCreation(workspaceId, userId, scope);
  return { ...draft, ...creation };
}
