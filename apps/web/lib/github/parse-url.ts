import { githubURLHost } from "./host";

/** Parse a GitHub repository URL with an explicit identity for private GHES. */
export function parseGitHubRepoUrl(
  input: string,
  expectedHost?: string,
): { owner: string; repo: string; host?: string } | null {
  const raw = input.trim();
  if (!raw) return null;
  try {
    const url = new URL(raw.includes("://") ? raw : `https://${raw}`);
    if (url.hostname === "www.github.com") url.hostname = "github.com";
    // Preserve legacy public share URL behavior; Enterprise requires HTTPS.
    if (url.protocol === "http:" && url.hostname === "github.com") url.protocol = "https:";
    const host = githubURLHost(url, expectedHost);
    if (!host || url.username || url.password) return null;
    const match = url.pathname.match(/^\/([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+?)(?:\.git)?\/?$/);
    return match
      ? { owner: match[1], repo: match[2], ...(host !== "github.com" ? { host } : {}) }
      : null;
  } catch {
    return null;
  }
}
