"use client";

import { useEffect, useReducer, useRef, type ClipboardEvent } from "react";
import { deleteAttachment, uploadAttachment } from "@/lib/api/domains/attachment-api";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import type { MessageAttachment } from "@/lib/services/session-launch-service";
import { generateUUID } from "@/lib/utils";
import { readClipboardAttachments } from "@/components/task/chat/clipboard-attachments";
import { MAX_FILES, MAX_TOTAL_SIZE } from "@/components/task/chat/file-attachment";
import {
  useAttachmentCountFeedback,
  useAttachmentFileFeedback,
  useAttachmentTotalSizeFeedback,
  useUnreadablePastedImageFeedback,
} from "@/components/task/chat/use-attachment-file-feedback";

export type CommentAttachmentOptions = {
  taskId: string;
  initial?: MessageAttachment[];
  onChange?: (attachments: MessageAttachment[]) => void;
};

type DraftFile = {
  key: string;
  name: string;
  status: "uploading" | "ready" | "failed";
  local?: File;
  descriptor?: MessageAttachment;
  expiresAt?: string;
};

function retainedFiles(initial: MessageAttachment[] | undefined): DraftFile[] {
  return (initial ?? []).flatMap((attachment) => {
    if (!attachment.attachment_id) return [];
    return [
      {
        key: attachment.attachment_id,
        name: attachment.name ?? attachment.attachment_id,
        status: "ready",
        descriptor: attachment,
      },
    ];
  });
}

function fileKind(file: File): MessageAttachment["type"] {
  if (file.type.startsWith("image/")) return "image";
  if (file.type.startsWith("audio/")) return "audio";
  return "resource";
}

function fileStatus(file: DraftFile): DraftFile["status"] {
  return file.expiresAt && Date.parse(file.expiresAt) <= Date.now() ? "failed" : file.status;
}

function release(id: string | undefined) {
  if (id) void deleteAttachment(id).catch(() => undefined);
}

function fileDescriptors(files: DraftFile[]) {
  return files.flatMap((file) => (file.descriptor ? [file.descriptor] : []));
}

function useAttachmentDraft(options?: CommentAttachmentOptions) {
  const draftRef = useRef<DraftFile[] | null>(null);
  const workspace = useRef<Promise<string> | null>(null);
  const live = useRef(false);
  const [, refresh] = useReducer((value: number) => value + 1, 0);
  if (!draftRef.current) draftRef.current = retainedFiles(options?.initial);
  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);

  const files = draftRef.current;
  useEffect(() => {
    const now = Date.now();
    const nextExpiry = Math.min(
      ...files.flatMap((file) => {
        const expiry = Date.parse(file.expiresAt ?? "");
        return expiry > now ? [expiry] : [];
      }),
    );
    if (!Number.isFinite(nextExpiry)) return;
    const timer = window.setTimeout(refresh, nextExpiry - now);
    return () => window.clearTimeout(timer);
  }, [files]);

  const update = (files: DraftFile[]) => {
    const previous = fileDescriptors(draftRef.current!);
    const next = fileDescriptors(files);
    draftRef.current = files;
    if (previous.length !== next.length || previous.some((file, index) => file !== next[index])) {
      options?.onChange?.(next);
    }
    refresh();
  };

  const upload = async (key: string, file: File) => {
    if (!options) return;
    const replace = (patch: Partial<DraftFile>) =>
      update(
        draftRef.current!.map((entry) => (entry.key === key ? { ...entry, ...patch } : entry)),
      );
    const active = () => live.current && draftRef.current!.some((entry) => entry.key === key);
    replace({ status: "uploading", expiresAt: undefined, descriptor: undefined });
    try {
      workspace.current ??= fetchTask(options.taskId, { cache: "no-store" }).then(
        (task) => task.workspace_id,
      );
      const workspaceId = await workspace.current;
      if (!active()) return;
      const uploaded = await uploadAttachment(file, {
        workspaceId,
        kind: fileKind(file),
        deliveryMode: "path",
      });
      if (!active()) {
        release(uploaded.attachment_id);
        return;
      }
      replace({
        status: "ready",
        expiresAt: uploaded.expires_at,
        descriptor: {
          attachment_id: uploaded.attachment_id,
          name: uploaded.name,
          type: uploaded.kind,
          mime_type: uploaded.mime_type,
          size_bytes: uploaded.size_bytes,
          delivery_mode: "path",
        },
      });
    } catch {
      workspace.current = null;
      if (active()) replace({ status: "failed" });
    }
  };

  return { files, update, upload, current: () => draftRef.current! };
}

export function useCommentAttachments(options?: CommentAttachmentOptions) {
  const { files, update, upload, current } = useAttachmentDraft(options);
  const countFeedback = useAttachmentCountFeedback();
  const fileFeedback = useAttachmentFileFeedback();
  const sizeFeedback = useAttachmentTotalSizeFeedback();
  const unreadablePaste = useUnreadablePastedImageFeedback();

  const addFiles = (incoming: File[]) => {
    const existing = current();
    const accepted = incoming.filter((file) => !fileFeedback(file));
    if (existing.length + accepted.length > MAX_FILES) return countFeedback();
    const bytes = existing.reduce(
      (sum, file) => sum + (file.local?.size ?? file.descriptor?.size_bytes ?? 0),
      0,
    );
    if (bytes + accepted.reduce((sum, file) => sum + file.size, 0) > MAX_TOTAL_SIZE) {
      return sizeFeedback();
    }
    const entries = accepted.map(
      (local): DraftFile => ({
        key: generateUUID(),
        name: local.name,
        status: "uploading",
        local,
      }),
    );
    update([...existing, ...entries]);
    for (const entry of entries) void upload(entry.key, entry.local!);
  };

  const handlePaste = (event: Pick<ClipboardEvent, "clipboardData" | "preventDefault">) => {
    if (!options) return;
    const { files: pasted, issue } = readClipboardAttachments(event.clipboardData);
    if (!pasted.length && !issue) return;
    if (!event.clipboardData.getData("text/plain").trim()) event.preventDefault();
    if (issue) unreadablePaste();
    else addFiles(pasted);
  };

  const remove = (key: string) => {
    const file = current().find((entry) => entry.key === key);
    if (file?.local) release(file.descriptor?.attachment_id);
    update(current().filter((entry) => entry.key !== key));
  };

  const retry = (key: string) => {
    const file = current().find((entry) => entry.key === key);
    if (file?.local && fileStatus(file) === "failed") {
      release(file.descriptor?.attachment_id);
      void upload(key, file.local);
    }
  };

  return {
    files: files.map((file) => ({ ...file, status: fileStatus(file) })),
    descriptors: fileDescriptors(files),
    ready: files.every((file) => fileStatus(file) === "ready"),
    canSubmit: () => current().every((file) => fileStatus(file) === "ready"),
    addFiles,
    handlePaste,
    remove,
    retry,
  };
}
