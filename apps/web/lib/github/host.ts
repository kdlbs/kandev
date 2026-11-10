export const PUBLIC_GITHUB_HOST = "github.com";

/** Canonical GitHub web authority. API destinations remain backend-owned. */
export function normalizeGitHubHost(input: string): string | null {
  if (!input || /[\s\u0000-\u001f\u007f]/.test(input) || /:\/?$/.test(input)) return null;
  try {
    const url = new URL(input.includes("://") ? input : `https://${input}`);
    if (
      url.protocol !== "https:" ||
      url.username ||
      url.password ||
      url.search ||
      url.hash ||
      url.pathname !== "/"
    )
      return null;
    const hostname = url.hostname.toLowerCase();
    if (!validDNSHostname(hostname) || !validGitHubWebAuthority(hostname, url.port)) return null;
    return url.host;
  } catch {
    return null;
  }
}

export function githubOrigin(host = PUBLIC_GITHUB_HOST): string | null {
  const authority = normalizeGitHubHost(host);
  return authority ? `https://${authority}` : null;
}

/** Recognize private GHES URLs only against a verified provider identity. */
export function githubURLHost(url: URL, expectedHost?: string): string | null {
  const authority = normalizeGitHubHost(url.origin);
  if (!authority) return null;
  if (authority === PUBLIC_GITHUB_HOST || /^[^.]+\.ghe\.com$/.test(authority)) return authority;
  return expectedHost && authority === normalizeGitHubHost(expectedHost) ? authority : null;
}

export function githubResourceKey(
  owner: string,
  repo: string,
  number: number,
  host = PUBLIC_GITHUB_HOST,
) {
  const authority = normalizeGitHubHost(host);
  const prefix = authority === PUBLIC_GITHUB_HOST ? "" : `${authority ?? host}/`;
  return `${prefix}${owner}/${repo}#${number}`;
}

function validDNSHostname(hostname: string) {
  return (
    Boolean(hostname) &&
    hostname.length <= 253 &&
    hostname.split(".").every((label) => /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label)) &&
    !/^\d+(?:\.\d+){3}$/.test(hostname)
  );
}
function validGitHubWebAuthority(hostname: string, port: string) {
  if (hostname === "api.github.com" || hostname === "ghe.com") return false;
  const dataResidency = hostname.endsWith(".ghe.com");
  if (dataResidency && !/^[^.]+\.ghe\.com$/.test(hostname)) return false;
  return !(port && (hostname === PUBLIC_GITHUB_HOST || dataResidency));
}
