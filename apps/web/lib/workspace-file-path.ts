function normalizeSeparators(path: string): string {
  return path.replaceAll("\\", "/");
}

function trimTrailingSeparators(path: string): string {
  if (path === "/" || /^[A-Za-z]:\/$/.test(path)) return path;
  return path.replace(/\/+$/, "");
}

function usesWindowsPathSemantics(path: string): boolean {
  return /^[A-Za-z]:\//.test(path);
}

function isAbsoluteFilePath(path: string): boolean {
  return (
    path.startsWith("/") ||
    usesWindowsPathSemantics(path) ||
    /^[A-Za-z][A-Za-z\d+.-]*:\/\//.test(path)
  );
}

/** Return whether a path is a canonical workspace-relative tree identity. */
export function isWorkspaceTreePath(path: string): boolean {
  if (path === "") return true;
  if (
    path.startsWith("/") ||
    path.includes("\\") ||
    path.includes("\0") ||
    /^[A-Za-z]:/.test(path) ||
    /^[A-Za-z][A-Za-z\d+.-]*:/.test(path)
  ) {
    return false;
  }
  return path.split("/").every((segment) => segment !== "" && segment !== "." && segment !== "..");
}

/** Return a workspace-relative alias for a contained path, or null when it is outside. */
export function workspaceRelativeFilePath(
  filePath: string,
  workspaceRoot: string | null | undefined,
): string | null {
  if (!workspaceRoot) return null;
  const path = trimTrailingSeparators(normalizeSeparators(filePath));
  const root = trimTrailingSeparators(normalizeSeparators(workspaceRoot));
  if (!root) return null;

  const windowsSemantics = usesWindowsPathSemantics(path) || usesWindowsPathSemantics(root);
  const comparablePath = windowsSemantics ? path.toLowerCase() : path;
  const comparableRoot = windowsSemantics ? root.toLowerCase() : root;
  if (comparablePath === comparableRoot) return "";
  const rootPrefix = comparableRoot.endsWith("/") ? comparableRoot : `${comparableRoot}/`;
  if (!comparablePath.startsWith(rootPrefix)) return null;
  const relativePath = path.slice(rootPrefix.length);
  return isWorkspaceTreePath(relativePath) ? relativePath : null;
}

/** Normalize a workspace-contained path while preserving external absolute paths. */
export function normalizeWorkspaceFilePath(
  filePath: string,
  workspaceRoot: string | null | undefined,
): string {
  const relativePath = workspaceRelativeFilePath(filePath, workspaceRoot);
  if (relativePath !== null) return relativePath;
  const normalizedPath = normalizeSeparators(filePath);
  return isAbsoluteFilePath(normalizedPath) ? filePath : normalizedPath;
}
