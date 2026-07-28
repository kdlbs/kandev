export type ImagePasteIssue = "unreadable-image";

export function readClipboardAttachments(clipboardData: DataTransfer): {
  files: File[];
  issue?: ImagePasteIssue;
} {
  const listedFiles = Array.from(clipboardData.files);
  if (listedFiles.length > 0) return { files: listedFiles };

  const itemFiles: File[] = [];
  for (const item of clipboardData.items) {
    if (item.kind !== "file") continue;
    const file = item.getAsFile();
    if (file) itemFiles.push(file);
  }
  if (itemFiles.length > 0) return { files: itemFiles };

  const hasImageItem = Array.from(clipboardData.items).some((item) =>
    item.type.startsWith("image/"),
  );
  if (hasImageItem || hasImageOnlyHtml(clipboardData)) {
    return { files: [], issue: "unreadable-image" };
  }
  return { files: [] };
}

function hasImageOnlyHtml(clipboardData: DataTransfer): boolean {
  if (clipboardData.getData("text/plain").trim()) return false;
  return /<img(?=[\t\n\f\r />])/i.test(clipboardData.getData("text/html"));
}
