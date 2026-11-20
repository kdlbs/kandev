"use client";

import { useCallback, useEffect, useMemo, useReducer, useRef } from "react";
import type {
  ClarificationAnswerAttachment,
  ClarificationRequestMetadata,
  Message,
} from "@/lib/types/http";
import { uploadAttachment, deleteAttachment } from "@/lib/api/domains/attachment-api";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import { generateUUID } from "@/lib/utils";
import { MAX_FILES, MAX_TOTAL_SIZE, processFile, type FileAttachment } from "./file-attachment";
import type { ImagePasteIssue } from "./clipboard-attachments";
import {
  useAttachmentCountFeedback,
  useAttachmentFileFeedback,
  useAttachmentTotalSizeFeedback,
  useUnreadablePastedImageFeedback,
} from "./use-attachment-file-feedback";

export type AnswerAttachmentFiles = Record<string, FileAttachment[]>;

export type ClarificationAnswerAttachmentController = {
  filesFor: (questionId: string) => FileAttachment[];
  addFiles: (questionId: string, files: File[], issue?: ImagePasteIssue) => void;
  remove: (questionId: string, fileId: string) => void;
  retry: (questionId: string, fileId: string) => void;
  readyAttachments: (questionId: string) => ClarificationAnswerAttachment[];
  clearAll: () => void;
  totalCount: number;
  failedCount: number;
  uploading: boolean;
  allReady: boolean;
};

const MAX_CACHED_BUNDLES = 8;
// Unmounted, unsubmitted drafts stay reusable for a while, then release their
// staged files so abandoned bundles cannot hold memory for the page lifetime.
const IDLE_STORE_TTL_MS = 15 * 60 * 1000;

type AttachmentFeedback = {
  count: () => void;
  fileTooLarge: (file: File) => boolean;
  total: () => void;
  unreadable: () => void;
};

type StoreSnapshot = {
  files: AnswerAttachmentFiles;
  processing: number;
  generation: number;
};

type AttachmentStore = {
  key: string;
  taskId: string | null;
  files: AnswerAttachmentFiles;
  initialized: Set<string>;
  processing: number;
  generation: number;
  workspaceId: string | null;
  workspacePromise: Promise<string | null> | null;
  uploads: Map<string, Promise<void>>;
  listeners: Set<() => void>;
  idleTimer: ReturnType<typeof setTimeout> | null;
  disposed: boolean;
  snapshot: StoreSnapshot;
  subscribe: (listener: () => void) => () => void;
};

const storesByBundle = new Map<string, AttachmentStore>();

function publish(store: AttachmentStore) {
  store.snapshot = {
    files: store.files,
    processing: store.processing,
    generation: store.generation,
  };
  for (const listener of store.listeners) listener();
}

function disposeStore(store: AttachmentStore) {
  store.disposed = true;
  if (store.idleTimer) {
    clearTimeout(store.idleTimer);
    store.idleTimer = null;
  }
  store.generation += 1;
  store.listeners.clear();
  for (const list of Object.values(store.files)) {
    for (const file of list) {
      if (file.file && file.attachmentId) {
        void deleteAttachment(file.attachmentId).catch(() => undefined);
      }
    }
  }
  if (storesByBundle.get(store.key) === store) storesByBundle.delete(store.key);
}

function getStore(bundleKey: string | null, taskId: string | null): AttachmentStore {
  if (!bundleKey) {
    return createStore("", taskId);
  }
  const existing = storesByBundle.get(bundleKey);
  if (existing && existing.taskId === taskId) {
    storesByBundle.delete(bundleKey);
    storesByBundle.set(bundleKey, existing);
    return existing;
  }
  if (existing) disposeStore(existing);
  const created = createStore(bundleKey, taskId);
  storesByBundle.set(bundleKey, created);
  while (storesByBundle.size > MAX_CACHED_BUNDLES) {
    let victimKey: string | undefined;
    for (const [key, candidate] of storesByBundle) {
      if (key !== bundleKey && candidate.listeners.size === 0) {
        victimKey = key;
        break;
      }
    }
    if (!victimKey) break;
    const victim = storesByBundle.get(victimKey);
    if (victim) disposeStore(victim);
  }
  return created;
}

function createStore(key: string, taskId: string | null): AttachmentStore {
  const store: AttachmentStore = {
    key,
    taskId,
    files: {},
    initialized: new Set(),
    processing: 0,
    generation: 0,
    workspaceId: null,
    workspacePromise: null,
    uploads: new Map(),
    listeners: new Set(),
    idleTimer: null,
    disposed: false,
    snapshot: { files: {}, processing: 0, generation: 0 },
    subscribe: (listener) => {
      store.listeners.add(listener);
      if (store.idleTimer) {
        clearTimeout(store.idleTimer);
        store.idleTimer = null;
      }
      return () => {
        store.listeners.delete(listener);
        scheduleIdleDispose(store);
      };
    },
  };
  return store;
}

function scheduleIdleDispose(store: AttachmentStore) {
  if (store.disposed || store.listeners.size > 0 || store.idleTimer) return;
  store.idleTimer = setTimeout(() => {
    store.idleTimer = null;
    if (!store.disposed && store.listeners.size === 0) disposeStore(store);
  }, IDLE_STORE_TTL_MS);
}

function setFiles(store: AttachmentStore, files: AnswerAttachmentFiles) {
  store.files = files;
  publish(store);
}

function countFiles(files: AnswerAttachmentFiles): number {
  return Object.values(files).reduce((total, list) => total + list.length, 0);
}

function totalFileBytes(files: AnswerAttachmentFiles): number {
  return Object.values(files).reduce(
    (total, list) => total + list.reduce((sum, file) => sum + file.size, 0),
    0,
  );
}

function attachmentKind(file: { mimeType: string }): "image" | "audio" | "resource" {
  if (file.mimeType.startsWith("image/")) return "image";
  if (file.mimeType.startsWith("audio/")) return "audio";
  return "resource";
}

function attachmentTypeFor(file: FileAttachment): ClarificationAnswerAttachment["type"] {
  if (file.isImage) return "image";
  if (file.mimeType.startsWith("audio/")) return "audio";
  return "resource";
}

function descriptorFor(file: FileAttachment): ClarificationAnswerAttachment | null {
  if (file.uploadStatus !== "ready" || !file.attachmentId) return null;
  return {
    attachment_id: file.attachmentId,
    type: attachmentTypeFor(file),
    name: file.fileName,
    mime_type: file.mimeType,
    size_bytes: file.size,
    delivery_mode: "path",
  };
}

function retainedFile(attachment: ClarificationAnswerAttachment): FileAttachment {
  return {
    id: attachment.attachment_id,
    attachmentId: attachment.attachment_id,
    mimeType: attachment.mime_type,
    fileName: attachment.name,
    size: attachment.size_bytes,
    isImage: attachment.type === "image",
    deliveryMode: "path",
    uploadStatus: "ready",
  };
}

function manifestFiles(messages: readonly Message[]): AnswerAttachmentFiles {
  const files: AnswerAttachmentFiles = {};
  for (const message of messages) {
    const metadata = message.metadata as ClarificationRequestMetadata | undefined;
    const questionId = metadata?.question_id ?? metadata?.question?.id;
    const attachments = metadata?.attachments;
    if (!questionId || !attachments?.length) continue;
    files[questionId] = attachments.map(retainedFile);
  }
  return files;
}

function bundleKeyFor(
  taskId: string | null,
  sessionId: string | null,
  pendingId: string | null,
): string | null {
  if (!pendingId) return null;
  return `${taskId ?? ""}::${sessionId ?? ""}::${pendingId}`;
}

function findFile(files: AnswerAttachmentFiles, questionId: string, fileId: string) {
  return files[questionId]?.find((file) => file.id === fileId);
}

function updateFile(
  store: AttachmentStore,
  questionId: string,
  fileId: string,
  update: (file: FileAttachment) => FileAttachment,
) {
  const list = store.files[questionId];
  if (!list) return;
  const nextList = list.map((file) => (file.id === fileId ? update(file) : file));
  setFiles(store, { ...store.files, [questionId]: nextList });
}

function removeFile(store: AttachmentStore, questionId: string, fileId: string) {
  const list = store.files[questionId];
  if (!list) return;
  setFiles(store, {
    ...store.files,
    [questionId]: list.filter((file) => file.id !== fileId),
  });
}

function ensureWorkspace(store: AttachmentStore): Promise<string | null> {
  if (store.workspaceId) return Promise.resolve(store.workspaceId);
  if (!store.taskId) return Promise.resolve(null);
  if (!store.workspacePromise) {
    const taskId = store.taskId;
    const request = fetchTask(taskId, { cache: "no-store" })
      .then((task) => task.workspace_id ?? null)
      .catch(() => null);
    store.workspacePromise = request;
    void request.finally(() => {
      if (store.workspacePromise === request) store.workspacePromise = null;
    });
  }
  const request = store.workspacePromise;
  return request.then((workspaceId) => {
    if (store.disposed) return null;
    if (workspaceId) store.workspaceId = workspaceId;
    return workspaceId;
  });
}

async function uploadFile(
  store: AttachmentStore,
  questionId: string,
  fileId: string,
): Promise<void> {
  if (store.disposed) return;
  const existing = store.uploads.get(fileId);
  if (existing) return existing;
  const file = findFile(store.files, questionId, fileId);
  if (!file?.file) return;
  const generation = store.generation;
  const promise = (async () => {
    const workspaceId = await ensureWorkspace(store);
    if (store.disposed || store.generation !== generation) return;
    if (!workspaceId) {
      if (findFile(store.files, questionId, fileId)) {
        updateFile(store, questionId, fileId, (current) => ({
          ...current,
          uploadStatus: "failed",
          uploadError: "workspace-unavailable",
        }));
      }
      return;
    }
    if (!findFile(store.files, questionId, fileId)) return;
    updateFile(store, questionId, fileId, (current) => ({
      ...current,
      uploadStatus: "uploading",
      uploadError: undefined,
    }));
    try {
      const descriptor = await uploadAttachment(file.file!, {
        workspaceId,
        kind: attachmentKind(file),
        deliveryMode: "path",
      });
      const stillExists = Boolean(findFile(store.files, questionId, fileId));
      if (store.disposed || store.generation !== generation || !stillExists) {
        await deleteAttachment(descriptor.attachment_id).catch(() => undefined);
        return;
      }
      updateFile(store, questionId, fileId, (current) => ({
        ...current,
        attachmentId: descriptor.attachment_id,
        expiresAt: descriptor.expires_at,
        mimeType: descriptor.mime_type,
        size: descriptor.size_bytes,
        isImage: descriptor.kind === "image",
        uploadStatus: "ready",
        uploadError: undefined,
      }));
    } catch {
      if (store.disposed || store.generation !== generation) return;
      if (!findFile(store.files, questionId, fileId)) return;
      updateFile(store, questionId, fileId, (current) => ({
        ...current,
        uploadStatus: "failed",
        uploadError: "upload-failed",
      }));
    }
  })();
  store.uploads.set(fileId, promise);
  void promise.finally(() => {
    if (store.uploads.get(fileId) === promise) store.uploads.delete(fileId);
  });
  return promise;
}

function placeholderFor(file: File): FileAttachment {
  const mimeType = file.type || "application/octet-stream";
  return {
    id: generateUUID(),
    file,
    mimeType,
    fileName: file.name,
    size: file.size,
    isImage: mimeType.startsWith("image/"),
    deliveryMode: "path",
    uploadStatus: "pending",
  };
}

function finishProcessing(store: AttachmentStore, questionId: string, fileId: string) {
  if (store.disposed) return;
  store.processing = Math.max(0, store.processing - 1);
  publish(store);
  void uploadFile(store, questionId, fileId);
}

function processPlaceholder(
  store: AttachmentStore,
  questionId: string,
  placeholder: FileAttachment,
  feedback: AttachmentFeedback,
) {
  const generation = store.generation;
  void processFile(placeholder.file as File).then((processed) => {
    if (store.disposed || store.generation !== generation) return;
    if (!findFile(store.files, questionId, placeholder.id)) {
      finishProcessing(store, questionId, placeholder.id);
      return;
    }
    if (!processed) {
      removeFile(store, questionId, placeholder.id);
      finishProcessing(store, questionId, placeholder.id);
      feedback.unreadable();
      return;
    }
    const current = findFile(store.files, questionId, placeholder.id);
    if (!current) {
      finishProcessing(store, questionId, placeholder.id);
      return;
    }
    if (countFiles(store.files) > MAX_FILES || totalFileBytes(store.files) > MAX_TOTAL_SIZE) {
      removeFile(store, questionId, placeholder.id);
      finishProcessing(store, questionId, placeholder.id);
      feedback.count();
      return;
    }
    updateFile(store, questionId, placeholder.id, (entry) => ({
      ...entry,
      data: processed.data,
      preview: processed.preview,
      mimeType: processed.mimeType,
      size: processed.size,
      isImage: processed.isImage,
      deliveryMode: "path",
      uploadStatus: "pending",
    }));
    finishProcessing(store, questionId, placeholder.id);
  });
}

function addFilesToStore(
  store: AttachmentStore,
  questionId: string,
  incoming: File[],
  feedback: AttachmentFeedback,
) {
  if (store.disposed || incoming.length === 0) return;
  const filtered = incoming.filter((file) => !feedback.fileTooLarge(file));
  if (filtered.length === 0) return;
  if (countFiles(store.files) + filtered.length > MAX_FILES) {
    feedback.count();
    return;
  }
  const addedBytes = filtered.reduce((total, file) => total + file.size, 0);
  if (totalFileBytes(store.files) + addedBytes > MAX_TOTAL_SIZE) {
    feedback.total();
    return;
  }
  store.initialized.add(questionId);
  const placeholders = filtered.map(placeholderFor);
  store.processing += placeholders.length;
  setFiles(store, {
    ...store.files,
    [questionId]: [...(store.files[questionId] ?? []), ...placeholders],
  });
  for (const placeholder of placeholders) {
    processPlaceholder(store, questionId, placeholder, feedback);
  }
}

function removeFromStore(store: AttachmentStore, questionId: string, fileId: string) {
  const file = findFile(store.files, questionId, fileId);
  if (!file) return;
  store.initialized.add(questionId);
  if (file.attachmentId && file.file) {
    void deleteAttachment(file.attachmentId).catch(() => undefined);
  }
  removeFile(store, questionId, fileId);
}

function retryInStore(store: AttachmentStore, questionId: string, fileId: string) {
  const file = findFile(store.files, questionId, fileId);
  if (!file?.file || store.uploads.has(fileId)) return;
  void uploadFile(store, questionId, fileId);
}

function mergeRetained(store: AttachmentStore, retained: AnswerAttachmentFiles) {
  if (store.disposed) return;
  let changed = false;
  const next = { ...store.files };
  for (const [questionId, list] of Object.entries(retained)) {
    if (store.initialized.has(questionId)) continue;
    store.initialized.add(questionId);
    if ((next[questionId]?.length ?? 0) > 0) continue;
    next[questionId] = list;
    changed = true;
  }
  if (changed) setFiles(store, next);
}

function sameAttachmentIDs(
  left: readonly ClarificationAnswerAttachment[] | undefined,
  right: readonly ClarificationAnswerAttachment[],
): boolean {
  const leftIDs = left ?? [];
  if (leftIDs.length !== right.length) return false;
  return leftIDs.every(
    (attachment, index) => attachment.attachment_id === right[index]?.attachment_id,
  );
}

// eslint-disable-next-line max-lines-per-function -- owns the question-scoped upload lifecycle.
export function useClarificationAnswerAttachments(options: {
  messages: readonly Message[];
  questionIds: readonly string[];
  pendingId: string | null;
}): ClarificationAnswerAttachmentController {
  const { messages, questionIds, pendingId } = options;
  const firstMessage = messages[0];
  const sessionId = firstMessage?.session_id ?? null;
  const taskId = firstMessage?.task_id ?? null;
  const bundleKey = bundleKeyFor(taskId, sessionId, pendingId);
  const store = useMemo(() => getStore(bundleKey, taskId), [bundleKey, taskId]);
  const [, forceRender] = useReducer((value: number) => value + 1, 0);
  const questionIdsRef = useRef(questionIds);
  const countFeedback = useAttachmentCountFeedback();
  const fileFeedback = useAttachmentFileFeedback();
  const totalSizeFeedback = useAttachmentTotalSizeFeedback();
  const unreadableImageFeedback = useUnreadablePastedImageFeedback();

  useEffect(() => {
    questionIdsRef.current = questionIds;
  }, [questionIds]);

  useEffect(() => {
    return store.subscribe(forceRender);
  }, [store]);

  useEffect(() => {
    if (!bundleKey) return;
    mergeRetained(store, manifestFiles(messages));
  }, [bundleKey, messages, store]);

  const snapshot = store.snapshot;
  const allFiles = useMemo(() => Object.values(snapshot.files).flat(), [snapshot.files]);
  const totalCount = allFiles.length;
  const failedCount = allFiles.filter((file) => file.uploadStatus === "failed").length;
  const uploading =
    snapshot.processing > 0 ||
    allFiles.some((file) => file.uploadStatus === "pending" || file.uploadStatus === "uploading");
  const allReady =
    failedCount === 0 && !uploading && allFiles.every((file) => file.uploadStatus === "ready");

  return {
    filesFor: useCallback(
      (questionId: string) => snapshot.files[questionId] ?? [],
      [snapshot.files],
    ),
    addFiles: useCallback(
      (questionId: string, incoming: File[], issue?: ImagePasteIssue) => {
        if (!questionIdsRef.current.includes(questionId)) return;
        if (issue === "unreadable-image") {
          unreadableImageFeedback();
          return;
        }
        addFilesToStore(store, questionId, incoming, {
          count: countFeedback,
          fileTooLarge: fileFeedback,
          total: totalSizeFeedback,
          unreadable: unreadableImageFeedback,
        });
      },
      [countFeedback, fileFeedback, store, totalSizeFeedback, unreadableImageFeedback],
    ),
    remove: useCallback(
      (questionId: string, fileId: string) => removeFromStore(store, questionId, fileId),
      [store],
    ),
    retry: useCallback(
      (questionId: string, fileId: string) => retryInStore(store, questionId, fileId),
      [store],
    ),
    readyAttachments: useCallback(
      (questionId: string): ClarificationAnswerAttachment[] =>
        (snapshot.files[questionId] ?? [])
          .map(descriptorFor)
          .filter((attachment): attachment is ClarificationAnswerAttachment => Boolean(attachment)),
      [snapshot.files],
    ),
    clearAll: useCallback(() => {
      store.generation += 1;
      store.processing = 0;
      for (const questionId of questionIdsRef.current) store.initialized.add(questionId);
      setFiles(store, {});
    }, [store]),
    totalCount,
    failedCount,
    uploading,
    allReady,
  };
}

export { sameAttachmentIDs };
