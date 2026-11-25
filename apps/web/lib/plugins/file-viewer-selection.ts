import { pluginRegistry } from "./registry";
import { getFileCategory, getFilePreviewKind } from "@/lib/utils/file-types";

export function fileViewerExtension(path: string): string {
  const name = path.replaceAll("\\", "/").split("/").pop() ?? "";
  const dot = name.lastIndexOf(".");
  const ext = dot < 0 ? "" : name.slice(dot + 1).toLowerCase();
  return /^[a-z0-9][a-z0-9_-]{0,31}$/.test(ext) ? ext : "";
}
export function fileViewerCandidates(path: string) {
  if (getFileCategory(path) === "image" || getFilePreviewKind(path) !== "none") return [];
  const ext = fileViewerExtension(path);
  return pluginRegistry.getFileViewers().filter((viewer) => viewer.extensions.includes(ext));
}
// i18n-exempt: source-code filename suffixes, not UI copy
const SOURCE_EXTENSIONS = new Set(
  "ts tsx js jsx mjs cjs go py rs java c cpp h hpp cs rb php sh bash zsh json yaml yml toml xml css scss sql vue svelte swift kt kts r lua dart ex exs erl hs pl ps1 bat cmd ini conf txt log csv tsv".split(
    " ",
  ),
);
export function shouldOpenFilePreview(path: string): boolean {
  const ext = fileViewerExtension(path);
  if (SOURCE_EXTENSIONS.has(ext)) return false;
  return (
    ext === "stl" ||
    getFileCategory(path) === "binary" ||
    (getFileCategory(path) !== "image" && fileViewerCandidates(path).length > 0)
  );
}
export function previewFileState(path: string, repo?: string) {
  // i18n-exempt: non-content hash sentinel for a metadata-only preview selection
  return {
    path,
    repo,
    name: path.replaceAll("\\", "/").split("/").pop() ?? path,
    content: "",
    originalContent: "",
    originalHash: "preview-only",
    isDirty: false,
    isBinary: true,
    previewOnly: true,
  };
}

export function canFindViewerForFileType(path: string): boolean {
  return (
    !!fileViewerExtension(path) &&
    !SOURCE_EXTENSIONS.has(fileViewerExtension(path)) &&
    getFileCategory(path) !== "image" &&
    getFilePreviewKind(path) === "none"
  );
}
