export function safeDecodePathSegment(segment: string | undefined): string | null {
  if (!segment) return null;
  try {
    return decodeURIComponent(segment);
  } catch {
    return null;
  }
}

export function matchSingle(pathname: string, pattern: RegExp): string | null {
  const match = pathname.match(pattern);
  return safeDecodePathSegment(match?.[1]);
}

export function matchDouble(pathname: string, pattern: RegExp): [string, string] | null {
  const match = pathname.match(pattern);
  if (!match?.[1] || !match[2]) return null;
  const first = safeDecodePathSegment(match[1]);
  const second = safeDecodePathSegment(match[2]);
  return first && second ? [first, second] : null;
}

export function normalizeSettingsPath(pathname: string): string {
  if (!pathname || pathname === "/settings/") return "/settings";
  return pathname.length > 1 && pathname.endsWith("/") ? pathname.slice(0, -1) : pathname;
}
