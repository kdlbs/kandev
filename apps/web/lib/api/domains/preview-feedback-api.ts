import { getWebSocketClient } from "@/lib/ws/connection";
import type {
  PreviewCaptureRect,
  PreviewElementSnapshot,
  PreviewTextAnchor,
  TaskPreviewFeedback,
  TaskPreviewFeedbackSnapshot,
} from "@/lib/types/http";

// i18n-exempt: programmer precondition; UI callers render localized errors.
const WS_CLIENT_UNAVAILABLE = "WebSocket client not available";

function requireClient() {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_CLIENT_UNAVAILABLE);
  return client;
}

export async function getTaskPreviewFeedback(taskId: string): Promise<TaskPreviewFeedbackSnapshot> {
  return (await requireClient().request("task.preview_feedback.list", {
    task_id: taskId,
  })) as TaskPreviewFeedbackSnapshot;
}

export type CreateTaskPreviewFeedbackInput = {
  taskId: string;
  id: string;
  kind: TaskPreviewFeedback["kind"];
  comment: string;
  sourceKind: TaskPreviewFeedback["source_kind"];
  sourceSessionId?: string;
  sourceLabel: string;
  sourcePath?: string;
  pageRoute: string;
  pageTitle: string;
  selectedText?: string;
  textAnchor?: PreviewTextAnchor;
  elementSnapshot?: PreviewElementSnapshot;
  captureRect?: PreviewCaptureRect;
  screenshotAttachmentId?: string;
};

export async function createTaskPreviewFeedback(
  input: CreateTaskPreviewFeedbackInput,
): Promise<TaskPreviewFeedbackSnapshot> {
  return (await requireClient().request("task.preview_feedback.create", {
    task_id: input.taskId,
    id: input.id,
    kind: input.kind,
    comment: input.comment,
    source_kind: input.sourceKind,
    source_session_id: input.sourceSessionId,
    source_label: input.sourceLabel,
    source_path: input.sourcePath,
    page_route: input.pageRoute,
    page_title: input.pageTitle,
    selected_text: input.selectedText,
    text_anchor: input.textAnchor,
    element_snapshot: input.elementSnapshot,
    capture_rect: input.captureRect,
    screenshot_attachment_id: input.screenshotAttachmentId,
  })) as TaskPreviewFeedbackSnapshot;
}

type ItemMutationInput = {
  taskId: string;
  id: string;
  expectedVersion: number;
};

export async function updateTaskPreviewFeedback(
  input: ItemMutationInput & { comment: string },
): Promise<TaskPreviewFeedbackSnapshot> {
  return (await requireClient().request("task.preview_feedback.update", {
    task_id: input.taskId,
    id: input.id,
    comment: input.comment,
    expected_version: input.expectedVersion,
  })) as TaskPreviewFeedbackSnapshot;
}

export async function deleteTaskPreviewFeedback(
  input: ItemMutationInput,
): Promise<TaskPreviewFeedbackSnapshot> {
  return (await requireClient().request("task.preview_feedback.delete", {
    task_id: input.taskId,
    id: input.id,
    expected_version: input.expectedVersion,
  })) as TaskPreviewFeedbackSnapshot;
}

export async function clearTaskPreviewFeedback(
  taskId: string,
  expectedRevision: number,
): Promise<TaskPreviewFeedbackSnapshot> {
  return (await requireClient().request("task.preview_feedback.clear", {
    task_id: taskId,
    expected_revision: expectedRevision,
  })) as TaskPreviewFeedbackSnapshot;
}
