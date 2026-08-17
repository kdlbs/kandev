import { getBackendConfig } from "@/lib/config";
import { readInterimSettingsInterlockToken } from "@/src/boot-payload";
import { ApiError } from "../client";

export type AttachmentDescriptor = {
  attachment_id: string;
  name: string;
  mime_type: string;
  kind: "image" | "audio" | "resource";
  delivery_mode: "prompt" | "path";
  size_bytes: number;
  state?: "staged" | "claimed" | "expired" | "deleted";
  expires_at?: string;
};

export async function uploadAttachment(
  file: File,
  params: {
    workspaceId: string;
    kind: AttachmentDescriptor["kind"];
    deliveryMode: AttachmentDescriptor["delivery_mode"];
  },
): Promise<AttachmentDescriptor> {
  const form = new FormData();
  form.append("workspace_id", params.workspaceId);
  form.append("kind", params.kind);
  form.append("delivery_mode", params.deliveryMode);
  form.append("mime_type", file.type || "application/octet-stream");
  form.append("file", file, file.name);
  const token = readInterimSettingsInterlockToken();
  const headers = token ? { ["X-Kandev-Interim-Settings-Interlock"]: token } : undefined;
  const response = await fetch(`${getBackendConfig().apiBaseUrl}/api/v1/attachments`, {
    method: "POST",
    body: form,
    credentials: "include",
    headers,
  });
  if (!response.ok) {
    let body: unknown = null;
    try {
      body = await response.json();
    } catch {
      // Keep the status as the useful error when the server returned no JSON.
    }
    // i18n-exempt: server-provided error text, or an HTTP status diagnostic when
    // the server sent none. Callers render translated copy for the generic case.
    const message =
      body && typeof body === "object" && "error" in body && typeof body.error === "string"
        ? body.error
        : `Attachment upload failed (${response.status})`;
    throw new ApiError(message, response.status, body);
  }
  return (await response.json()) as AttachmentDescriptor;
}

export function attachmentContentUrl(attachmentId: string): string {
  return `${getBackendConfig().apiBaseUrl}/api/v1/attachments/${encodeURIComponent(attachmentId)}/content`;
}

export async function deleteAttachment(attachmentId: string): Promise<void> {
  const token = readInterimSettingsInterlockToken();
  const response = await fetch(
    `${getBackendConfig().apiBaseUrl}/api/v1/attachments/${encodeURIComponent(attachmentId)}`,
    {
      method: "DELETE",
      credentials: "include",
      headers: token ? { ["X-Kandev-Interim-Settings-Interlock"]: token } : undefined,
    },
  );
  if (!response.ok && response.status !== 404) {
    throw new ApiError(`Attachment delete failed (${response.status})`, response.status, null);
  }
}
