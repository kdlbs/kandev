/**
 * Browser-side defense-in-depth validation for the provider remediation URL
 * contract (ADR-2026-08-07-allowlisted-provider-action-links). The backend
 * already allowlists the URL; this mirror rejects anything unexpected before
 * it becomes an href, so a malformed or hostile metadata value can never
 * render as a navigable link.
 *
 * The raw string must already be the exact allowlisted route. Parsing is a
 * secondary validation/error step — never evidence that the route is
 * allowlisted — so literal or percent-encoded traversal cannot normalize
 * into a passing shape.
 */

const REMEDIATION_URL_MAX_LENGTH = 256;
const REMEDIATION_URL_PATTERN = /^https:\/\/opencode\.ai\/workspace\/[A-Za-z0-9_-]{1,128}\/go$/;

export function normalizeRemediationUrl(raw: unknown): string | null {
  if (typeof raw !== "string" || raw.length === 0 || raw.length > REMEDIATION_URL_MAX_LENGTH) {
    return null;
  }
  if (!REMEDIATION_URL_PATTERN.test(raw)) {
    return null;
  }
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return null;
  }
  // Defense in depth: the parsed shape must agree with the raw allowlist.
  if (url.protocol !== "https:" || url.hostname !== REMEDIATION_URL_HOST) {
    return null;
  }
  if (
    url.username !== "" ||
    url.password !== "" ||
    url.search !== "" ||
    url.hash !== "" ||
    url.port !== ""
  ) {
    return null;
  }
  return url.href;
}

const REMEDIATION_URL_HOST = "opencode.ai";
