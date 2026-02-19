/** Check if a path looks like a directory (ends with / or has no extension). */
export function isDirectory(filePath: string): boolean {
  if (filePath.endsWith("/")) return true;
  const name = filePath.split("/").pop() || "";
  return !name.includes(".");
}

/** Extract filename from a path. */
export function getFileName(filePath: string): string {
  return filePath.split("/").pop() || filePath;
}
