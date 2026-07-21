export function normalizeGitLabOrigin(rawHost: string): string {
  try {
    const parsed = new URL(rawHost.includes("://") ? rawHost : `https://${rawHost}`);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return "";
    return `${parsed.protocol}//${parsed.host}`.toLowerCase();
  } catch {
    return "";
  }
}

export function gitLabMRKey(host: string, projectPath: string, iid: number): string {
  const path = projectPath
    .trim()
    .replace(/^\/+|\/+$/g, "")
    .toLowerCase();
  return `${normalizeGitLabOrigin(host)}|${path}!${iid}`;
}
