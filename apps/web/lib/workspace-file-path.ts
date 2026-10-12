function normalizeSeparators(path: string): string {
  return path.replaceAll("\\", "/");
}

function trimTrailingSeparators(path: string): string {
  if (path === "/" || /^[A-Za-z]:\/$/.test(path) || /^file:\/\/\/(?:[A-Za-z]:\/)?$/i.test(path)) {
    return path;
  }
  return path.replace(/\/+$/, "");
}

function usesWindowsPathSemantics(path: string): boolean {
  return (
    /^[A-Za-z]:\//.test(path) ||
    path.startsWith("//") ||
    /^file:\/\/\/[A-Za-z]:(?:\/|$)/i.test(path) ||
    /^file:\/\/[^/]/i.test(path)
  );
}

function hasAbsoluteURIForm(path: string): boolean {
  return /^[A-Za-z][A-Za-z\d+.-]*:\/\//.test(path) || /^file:\//i.test(path);
}

function isAbsoluteFilePath(path: string): boolean {
  return path.startsWith("/") || usesWindowsPathSemantics(path) || hasAbsoluteURIForm(path);
}

/** Return whether a path is a canonical workspace-relative tree identity. */
export function isWorkspaceTreePath(path: string): boolean {
  if (path === "") return true;
  if (
    path.startsWith("/") ||
    path.includes("\\") ||
    path.includes("\0") ||
    /^[A-Za-z]:/.test(path) ||
    hasAbsoluteURIForm(path)
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
  const comparablePath = windowsSemantics ? path.toLocaleLowerCase("en-US") : path;
  const comparableRoot = windowsSemantics ? root.toLocaleLowerCase("en-US") : root;
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
