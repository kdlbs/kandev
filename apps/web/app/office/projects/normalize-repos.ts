// normalizeRepos accepts the `repositories` field as it actually arrives
// on the wire. After the JSON-shape fix it should always be string[], but
// stale SSR caches and legacy entries persisted as a JSON-encoded string
// fall back through the parse path so callers never crash on either shape.
export function normalizeRepos(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.filter((v): v is string => typeof v === "string");
  }
  if (typeof value === "string" && value.trim() !== "") {
    try {
      const parsed = JSON.parse(value);
      if (Array.isArray(parsed)) {
        return parsed.filter((v): v is string => typeof v === "string");
      }
    } catch {
      // Fall through to empty array.
    }
  }
  return [];
}
