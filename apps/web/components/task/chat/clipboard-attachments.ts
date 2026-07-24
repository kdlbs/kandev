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
  if (hasImageItem || hasImageOnlyHtml(clipboardData.getData("text/html"))) {
    return { files: [], issue: "unreadable-image" };
  }
  return { files: [] };
}

function hasImageOnlyHtml(html: string): boolean {
  if (!html) return false;
  const parsed = new DOMParser().parseFromString(html, "text/html");
  if (!parsed.body.querySelector("img")) return false;
  parsed.body
    .querySelectorAll("img, script, style, template, noscript")
    .forEach((element) => element.remove());
  return !parsed.body.textContent?.trim();
}
