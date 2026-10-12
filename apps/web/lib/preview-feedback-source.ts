const SENSITIVE_QUERY_PARAMETER =
  /(?:^|[_-])(access[_-]?token|id[_-]?token|token|secret|password|passwd|auth(?:orization)?|credential|cookie|session|capability|signature|sig|nonce|code|state|key)(?:$|[_-])/i;

function safeSearch(search: string): string {
  const kept: string[] = [];
  const params = new URLSearchParams(search);
  params.forEach((value, name) => {
    if (SENSITIVE_QUERY_PARAMETER.test(name)) return;
    kept.push(`${encodeURIComponent(name)}=${encodeURIComponent(value)}`);
  });
  return kept.length ? `?${kept.join("&")}` : "";
}

/** Returns only the stable service identity for a browser preview source. */
export function previewSourceLabel(value: string): string {
  try {
    const url = new URL(value);
    if (url.origin !== "null") return url.origin;
  } catch {
    // The browser preview normally supplies an absolute URL. Keep the
    // fallback below defensive for malformed or partially typed input.
  }
  return sanitizePreviewPageRoute(value).split("?")[0];
}

/** Keeps the path and non-sensitive query parameters of a rendered page route. */
export function sanitizePreviewPageRoute(value: string): string {
  try {
    const url = new URL(value, "http://kandev.invalid");
    return `${url.pathname || "/"}${safeSearch(url.search)}`;
  } catch {
    return value.split("#", 1)[0].split("?", 1)[0] || "/";
  }
}
