export type FileCategory = "text" | "image" | "binary";

const IMAGE_EXTENSIONS = new Set([
  "png",
  "jpg",
  "jpeg",
  "gif",
  "svg",
  "webp",
  "ico",
  "bmp",
  "avif",
]);

const BINARY_EXTENSIONS = new Set([
  // Executables & libraries
  "exe",
  "dll",
  "so",
  "dylib",
  "o",
  "a",
  // Archives
  "zip",
  "tar",
  "gz",
  "bz2",
  "7z",
  "rar",
  "xz",
  "zst",
  // Fonts
  "woff",
  "woff2",
  "ttf",
  "otf",
  "eot",
  // Media
  "mp3",
  "mp4",
  "wav",
  "ogg",
  "flac",
  "avi",
  "mov",
  "mkv",
  "webm",
  // Documents
  "pdf",
  "doc",
  "docx",
  "xls",
  "xlsx",
  "ppt",
  "pptx",
  // Compiled / data
  "pyc",
  "pyo",
  "class",
  "wasm",
  "sqlite",
  "db",
  // Images already covered above, but some binary-only image formats
  "psd",
  "tiff",
  "tif",
]);

const MIME_MAP: Record<string, string> = {
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  gif: "image/gif",
  svg: "image/svg+xml",
  webp: "image/webp",
  ico: "image/x-icon",
  bmp: "image/bmp",
  avif: "image/avif",
};

function getExtension(path: string): string {
  const dot = path.lastIndexOf(".");
  if (dot === -1) return "";
  return path.slice(dot + 1).toLowerCase();
}

/**
 * Determine the file category based on extension.
 * Note: the backend's `is_binary` flag is the source of truth for encoding;
 * this only determines which viewer to use.
 */
export function getFileCategory(path: string): FileCategory {
  const ext = getExtension(path);
  if (IMAGE_EXTENSIONS.has(ext)) return "image";
  if (BINARY_EXTENSIONS.has(ext)) return "binary";
  return "text";
}

/** Return the MIME type for a known image extension, or a fallback. */
export function getImageMimeType(path: string): string {
  const ext = getExtension(path);
  return MIME_MAP[ext] || "application/octet-stream";
}
