export type TriggerFileDownloadParams = {
  fileName: string;
  content: string;
  isBinary: boolean;
};

/** Return the last path segment of a POSIX or Windows path. */
export function fileBasename(path: string): string {
  const idx = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return idx >= 0 ? path.slice(idx + 1) : path;
}

function base64ToArrayBuffer(base64: string): ArrayBuffer {
  const binary = atob(base64);
  const buffer = new ArrayBuffer(binary.length);
  const view = new Uint8Array(buffer);
  for (let i = 0; i < binary.length; i++) view[i] = binary.charCodeAt(i);
  return buffer;
}

/**
 * Trigger a browser download for the given file content by creating a Blob
 * and clicking a temporary anchor. Binary content is expected to be
 * base64-encoded (matches the workspace.file.get contract).
 */
export function triggerFileDownload({
  fileName,
  content,
  isBinary,
}: TriggerFileDownloadParams): void {
  const blob = isBinary
    ? new Blob([base64ToArrayBuffer(content)], { type: "application/octet-stream" })
    : new Blob([content], { type: "text/plain;charset=utf-8" });

  const url = URL.createObjectURL(blob);
  try {
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = fileBasename(fileName);
    anchor.rel = "noopener";
    document.body.appendChild(anchor);
    anchor.click();
    document.body.removeChild(anchor);
  } finally {
    URL.revokeObjectURL(url);
  }
}
