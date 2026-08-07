/**
 * Browser-side defense-in-depth validation for the provider remediation URL
 * contract (ADR-2026-08-07-allowlisted-provider-action-links). The backend
 * already allowlists the URL; this mirror rejects anything unexpected before
 * it becomes an href, so a malformed or hostile metadata value can never
 * render as a navigable link.
 */

const REMEDIATION_URL_MAX_LENGTH = 256;
const REMEDIATION_URL_HOST = "opencode.ai";
const REMEDIATION_URL_PREFIX = "/workspace/";
const REMEDIATION_URL_SUFFIX = "/go";
const WORKSPACE_ID_PATTERN = /^[A-Za-z0-9_-]+$/;

export function normalizeRemediationUrl(raw: unknown): string | null {
  if (typeof raw !== "string" || raw.length === 0 || raw.length > REMEDIATION_URL_MAX_LENGTH) {
    return null;
  }
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return null;
  }
  if (!hasAllowlistedAuthority(raw, url)) {
    return null;
  }
  if (
    !url.pathname.startsWith(REMEDIATION_URL_PREFIX) ||
    !url.pathname.endsWith(REMEDIATION_URL_SUFFIX)
  ) {
    return null;
  }
  const workspaceId = url.pathname.slice(
    REMEDIATION_URL_PREFIX.length,
    url.pathname.length - REMEDIATION_URL_SUFFIX.length,
  );
  if (
    workspaceId.length === 0 ||
    workspaceId.length > 128 ||
    !WORKSPACE_ID_PATTERN.test(workspaceId)
  ) {
    return null;
  }
  return url.href;
}

/**
 * The authority must be exactly `opencode.ai` — an explicit default port
 * (`:443`) survives URL parsing but is not part of the allowlisted shape.
 */
function hasAllowlistedAuthority(raw: string, url: URL): boolean {
  if (!raw.startsWith(`https://${REMEDIATION_URL_HOST}/`)) {
    return false;
  }
  return (
    url.protocol === "https:" &&
    url.username === "" &&
    url.password === "" &&
    url.search === "" &&
    url.hash === "" &&
    url.port === "" &&
    url.hostname === REMEDIATION_URL_HOST
  );
}
