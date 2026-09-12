export const TASK_DESCRIPTION_SYNTHETIC_ID = "task-description";

type PreviewAttachment = {
  attachment_id: string;
  type: "image" | "resource";
  mime_type: string;
  name: string;
  size_bytes?: number;
  delivery_mode?: "prompt" | "path";
};

export type InitialPromptPreview = {
  content: string;
  attachments: PreviewAttachment[];
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function readAttachment(value: unknown): PreviewAttachment | null {
  if (
    !isRecord(value) ||
    typeof value.attachment_id !== "string" ||
    !value.attachment_id.trim() ||
    !["image", "resource"].includes(value.type as string) ||
    typeof value.mime_type !== "string" ||
    typeof value.name !== "string"
  ) {
    return null;
  }
  return {
    attachment_id: value.attachment_id,
    type: value.type as PreviewAttachment["type"],
    mime_type: value.mime_type,
    name: value.name,
    ...(typeof value.size_bytes === "number" &&
    Number.isFinite(value.size_bytes) &&
    value.size_bytes > 0
      ? { size_bytes: value.size_bytes }
      : {}),
    ...(value.delivery_mode === "prompt" || value.delivery_mode === "path"
      ? { delivery_mode: value.delivery_mode }
      : {}),
  };
}

export function readInitialPromptPreview(value: unknown): InitialPromptPreview | null {
  if (!isRecord(value)) return null;
  const content = typeof value.content === "string" ? value.content : "";
  const attachments = Array.isArray(value.attachments)
    ? value.attachments
        .slice(0, 10)
        .map(readAttachment)
        .filter((item) => item !== null)
    : [];
  return content.trim() || attachments.length ? { content, attachments } : null;
}
