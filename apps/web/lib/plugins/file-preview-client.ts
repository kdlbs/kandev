import { fetchJson, fetchResponse } from "@/lib/api/client";

export type PreviewPolicy = { mode: "ask" | "allow" | "deny"; revision: number };
export type PreviewSelection = {
  id: string;
  path: string;
  original_path: string;
  size_bytes: number;
  external: boolean;
  execution_label: string;
  approval_required: boolean;
};
export type PreviewContext = { pluginId: string; workspaceId: string };
// i18n-exempt: internal protocol failure, callers render localized recovery copy
const PREVIEW_ERROR = "file_preview_unavailable";
function endpoint(context: PreviewContext, suffix: string) {
  return `/api/plugins/${encodeURIComponent(context.pluginId)}/${suffix}?workspace_id=${encodeURIComponent(context.workspaceId)}`;
}
export function getPreviewPolicy(context: PreviewContext, signal?: AbortSignal) {
  return fetchJson<PreviewPolicy>(endpoint(context, "file-preview-policy"), {
    cache: "no-store",
    init: { signal },
  });
}
export async function setPreviewPolicy(
  context: PreviewContext,
  mode: PreviewPolicy["mode"],
  revision: number,
) {
  const result = await fetchJson<PreviewPolicy>(endpoint(context, "file-preview-policy"), {
    init: {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ mode, expected_revision: revision }),
    },
  });
  window.dispatchEvent(new Event("kandev-preview-policy-changed"));
  return result;
}
export function preparePreview(
  context: PreviewContext,
  input: { session_id: string; path: string; repo?: string; viewer_id: string },
  signal: AbortSignal,
) {
  return fetchJson<PreviewSelection>(endpoint(context, "file-previews"), {
    init: {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
      signal,
    },
  });
}
export async function cancelPreview(context: PreviewContext, id: string) {
  await fetchJson(endpoint(context, `file-previews/${encodeURIComponent(id)}`), {
    init: { method: "DELETE" },
  });
}
export async function readPreview(
  context: PreviewContext,
  selection: PreviewSelection,
  signal: AbortSignal,
) {
  const response = await fetchResponse(
    endpoint(context, `file-previews/${encodeURIComponent(selection.id)}/read`),
    {
      cache: "no-store",
      init: {
        method: "POST",
        redirect: "error",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ approved: true }),
        signal,
      },
    },
  );
  return consumePreviewResponse(response, selection.size_bytes, signal);
}

export async function consumePreviewResponse(
  response: Response,
  size: number,
  signal: AbortSignal,
): Promise<ArrayBuffer> {
  signal.throwIfAborted();
  if (
    !response.ok ||
    !Number.isSafeInteger(size) ||
    size < 0 ||
    size > 64 * 1024 * 1024 ||
    !response.body
  )
    throw new Error(PREVIEW_ERROR);
  const reader = response.body.getReader();
  const bytes = new Uint8Array(size);
  let received = 0;
  let complete = false;
  try {
    for (;;) {
      signal.throwIfAborted();
      const { value, done } = await reader.read();
      signal.throwIfAborted();
      if (done) break;
      if (received + value.length > size + 1) throw new Error(PREVIEW_ERROR);
      const dataLength = Math.min(value.length, Math.max(0, size - received));
      bytes.set(value.subarray(0, dataLength), Math.min(received, size));
      if (dataLength < value.length) complete = value[dataLength] === 0;
      received += value.length;
    }
    if (received !== size + 1 || !complete) throw new Error(PREVIEW_ERROR);
    return bytes.buffer;
  } finally {
    await reader.cancel();
    reader.releaseLock();
  }
}
