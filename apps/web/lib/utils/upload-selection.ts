/**
 * Normalizes a browser file selection into destination-relative paths.
 *
 * A flat multi-file pick yields bare names. A folder pick (an input carrying
 * `webkitdirectory`) yields `webkitRelativePath`, which already includes the
 * chosen folder as its first segment. Both collapse to the same shape so the
 * upload hook, the conflict dialog, and the API client never branch on how the
 * files were chosen.
 */
export type UploadSelectionEntry = {
  file: File;
  relativePath: string;
};

export type UploadSelection = {
  entries: UploadSelectionEntry[];
  /** Files that could not be read, reported so one bad entry is not silent. */
  skipped: string[];
};

/**
 * Strip a leading slash, collapse `.` segments, and reject anything that tries
 * to climb out of the destination. The backend rejects these too; refusing here
 * keeps a malformed pick from costing a round trip.
 */
function sanitizeRelativePath(raw: string): string | null {
  const normalized = raw.replace(/\\/g, "/");
  if (!normalized || normalized.startsWith("/")) return null;

  const segments: string[] = [];
  for (const segment of normalized.split("/")) {
    if (segment === "" || segment === ".") continue;
    if (segment === "..") return null;
    segments.push(segment);
  }
  return segments.length > 0 ? segments.join("/") : null;
}

/**
 * Normalize a `FileList` (or array) from either picker into upload entries.
 *
 * `webkitRelativePath` is empty for a flat pick and populated for a directory
 * pick, so one code path serves both.
 */
export function normalizeUploadSelection(files: ArrayLike<File>): UploadSelection {
  const entries: UploadSelectionEntry[] = [];
  const skipped: string[] = [];

  for (let i = 0; i < files.length; i++) {
    const file = files[i];
    if (!file) continue;

    const candidate = file.webkitRelativePath ? file.webkitRelativePath : file.name;
    const relativePath = sanitizeRelativePath(candidate);
    if (!relativePath) {
      skipped.push(candidate || file.name);
      continue;
    }
    entries.push({ file, relativePath });
  }

  return { entries, skipped };
}

/** Join a destination folder and a relative path into a workspace path. */
export function joinDestination(dir: string, relativePath: string): string {
  const cleanDir = dir.replace(/^\/+|\/+$/g, "");
  return cleanDir ? `${cleanDir}/${relativePath}` : relativePath;
}
