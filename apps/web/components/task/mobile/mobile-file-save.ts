"use client";

import { useCallback, useContext, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import { getWebSocketClient } from "@/lib/ws/connection";
import { updateFileContent } from "@/lib/ws/workspace-files";
import { generateUnifiedDiff } from "@/lib/utils/file-diff";
import type { OpenFileTab } from "@/lib/types/backend";
import { FileDraftSessionContext, type FileDraftSessionOwner } from "../file-draft-session-context";
import {
  getMobileFileInstanceId,
  type MobileFileSavedSnapshot,
} from "./mobile-selected-file-state";

type MobileFileSaveOptions = {
  file: OpenFileTab;
  sessionId: string | null;
  fileIdentity: string;
  draftContent: string;
  baselineContent: string;
  originalHash: string;
  isDirty: boolean;
  markSaved: (expectedFileIdentity: string, content: string, hash: string) => void;
  onFileSaved?: (snapshot: MobileFileSavedSnapshot) => void;
};

function captureSaveOwner(file: OpenFileTab, retainedOwner: FileDraftSessionOwner | null) {
  const fileKey = buildRepoScopedItemId(file.path, file.repo);
  const state = useDockviewStore.getState();
  const instanceId = getMobileFileInstanceId(file);
  if (instanceId && state.openFiles.get(fileKey)?.instanceId !== instanceId) return null;
  const ownerVisit = retainedOwner?.visitRef.current;
  const epoch = state.fileSessionEpoch;
  const operation = Symbol();
  if (instanceId) state.updateFileState(fileKey, { saveOperation: operation });
  return {
    fileKey,
    isCanonical: !!instanceId,
    isCurrent: () => {
      if (!instanceId) return true;
      const latest = useDockviewStore.getState();
      const draft = latest.openFiles.get(fileKey);
      return (
        (!retainedOwner || (!!ownerVisit && retainedOwner.visitRef.current === ownerVisit)) &&
        latest.fileSessionEpoch === epoch &&
        draft?.instanceId === instanceId &&
        draft.saveOperation === operation
      );
    },
  };
}

function publishCanonicalSave(fileKey: string, content: string, originalHash: string) {
  const latest = useDockviewStore.getState();
  latest.updateFileState(fileKey, {
    originalContent: content,
    originalHash,
    isDirty: latest.openFiles.get(fileKey)?.content !== content,
    hasRemoteUpdate: false,
    remoteContent: undefined,
    remoteOriginalHash: undefined,
  });
}

export function useMobileFileSave(options: MobileFileSaveOptions) {
  const { t } = useTranslation();
  const retainedOwner = useContext(FileDraftSessionContext);
  const { toast } = useToast();
  const [isSaving, setIsSaving] = useState(false);
  const handleSave = useCallback(async () => {
    const { file, sessionId, draftContent, baselineContent, originalHash, isDirty } = options;
    if (!sessionId || !isDirty || isSaving) return;
    const client = getWebSocketClient();
    if (!client) return;
    const owner = captureSaveOwner(file, retainedOwner);
    if (!owner) return;
    setIsSaving(true);
    try {
      const response = await updateFileContent(client, sessionId, {
        path: file.path,
        diff: generateUnifiedDiff(baselineContent, draftContent, file.path),
        originalHash,
        desiredContent: draftContent,
        repo: file.repo,
      });
      if (!owner.isCurrent()) return;
      if (response.success && response.new_hash) {
        if (owner.isCanonical) {
          publishCanonicalSave(owner.fileKey, draftContent, response.new_hash);
          return;
        }
        options.markSaved(options.fileIdentity, draftContent, response.new_hash);
        options.onFileSaved?.({
          path: file.path,
          repo: file.repo,
          sessionId,
          content: draftContent,
          originalContent: draftContent,
          originalHash: response.new_hash,
        });
        return;
      }
      toast({
        title: t("editors:saveFailed"),
        description: response.error || t("editors:failedToSaveFile"),
        variant: "error",
      });
    } catch (error) {
      if (!owner.isCurrent()) return;
      toast({
        title: t("editors:saveFailed"),
        description: error instanceof Error ? error.message : t("editors:errorWhileSavingFile"),
        variant: "error",
      });
    } finally {
      setIsSaving(false);
    }
  }, [options, isSaving, retainedOwner, t, toast]);
  return { isSaving, handleSave };
}
