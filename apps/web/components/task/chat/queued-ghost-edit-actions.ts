import { useCallback } from "react";
import type { EntityReference } from "@/lib/types/entity-reference";
import { QueueEntryNotFoundError } from "@/lib/api/domains/queue-api";
import { queueMutationErrorKey } from "@/lib/queue-mutation-error";
import { toast } from "@/lib/toast/sonner";
import { survivingEntityReferences } from "@/lib/entity-references/message-references";
import { queueReferenceSelectionsChanged } from "@/hooks/domains/session/queue-registration";
import type { QueuedAttachment } from "./queued-attachment-row";

type QueuedGhostSaveArgs = {
  value: string;
  entryContent: string;
  originalReferences: readonly EntityReference[];
  entityReferences: readonly EntityReference[];
  attachments?: QueuedAttachment[];
  onSave: (
    content: string,
    references: EntityReference[],
    attachments?: QueuedAttachment[],
  ) => void | Promise<void>;
  onEditComplete?: (editToken?: string, saved?: boolean) => void | Promise<void>;
  onSaveResult?: (success: boolean) => void;
  editTokenRef: React.RefObject<string | undefined>;
  setEditing: (editing: boolean) => void;
  setSaving: (saving: boolean) => void;
  t: (key: string) => string;
};

async function saveQueuedGhostValue(
  onSave: QueuedGhostSaveArgs["onSave"],
  value: string,
  references: EntityReference[],
  attachments?: QueuedAttachment[],
) {
  if (attachments === undefined) {
    await onSave(value, references);
  } else {
    await onSave(value, references, attachments);
  }
}

export function useQueuedGhostSave({
  value,
  entryContent,
  originalReferences,
  entityReferences,
  attachments,
  onSave,
  onEditComplete,
  onSaveResult,
  editTokenRef,
  setEditing,
  setSaving,
  t,
}: QueuedGhostSaveArgs) {
  return useCallback(async () => {
    const editToken = editTokenRef.current;
    const trimmed = value.trim();
    const hasAttachments = (attachments?.length ?? 0) > 0;
    const updatedReferences = survivingEntityReferences(trimmed, entityReferences);
    const referencesChanged = queueReferenceSelectionsChanged(
      originalReferences,
      updatedReferences,
    );
    if ((!trimmed && !hasAttachments) || (trimmed === entryContent && !referencesChanged)) {
      onSaveResult?.(true);
      setEditing(false);
      await onEditComplete?.(editToken, false);
      return;
    }
    setSaving(true);
    try {
      await saveQueuedGhostValue(onSave, trimmed, updatedReferences, attachments);
      onSaveResult?.(true);
      setEditing(false);
      await onEditComplete?.(editToken, true);
    } catch (err) {
      onSaveResult?.(false);
      console.error("Failed to update queued entry:", err);
      if (err instanceof QueueEntryNotFoundError) {
        toast.error(t("chat:queueEditAlreadySent"));
      } else {
        toast.error(t(queueMutationErrorKey(err, "chat:queueEditSaveFailed")));
      }
      setEditing(false);
      await onEditComplete?.(editToken, false);
    } finally {
      setSaving(false);
    }
  }, [
    value,
    entryContent,
    originalReferences,
    onSave,
    onEditComplete,
    onSaveResult,
    entityReferences,
    attachments,
    editTokenRef,
    setEditing,
    setSaving,
    t,
  ]);
}

type QueuedGhostCancelArgs = {
  entryContent: string;
  entityReferences: readonly EntityReference[];
  onEditComplete?: (editToken?: string, saved?: boolean) => void | Promise<void>;
  editTokenRef: React.RefObject<string | undefined>;
  setValue: (value: string) => void;
  setReferences: (references: EntityReference[]) => void;
  setEditing: (editing: boolean) => void;
};

export function useQueuedGhostCancel({
  entryContent,
  entityReferences,
  onEditComplete,
  editTokenRef,
  setValue,
  setReferences,
  setEditing,
}: QueuedGhostCancelArgs) {
  return useCallback(async () => {
    const editToken = editTokenRef.current;
    setValue(entryContent);
    setReferences([...entityReferences]);
    setEditing(false);
    await onEditComplete?.(editToken);
  }, [
    editTokenRef,
    entryContent,
    entityReferences,
    onEditComplete,
    setEditing,
    setReferences,
    setValue,
  ]);
}
