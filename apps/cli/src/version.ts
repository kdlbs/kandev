/**
 * Centralized version parsing and comparison for the CLI.
 *
 * Version format: optional "v" prefix + dot-separated numeric segments.
 * Examples: "v1.2", "0.3.0", "v2.0.1", "1"
 */

export type ParsedVersion = {
  raw: string;
  segments: number[];
};

/**
 * Parse a version string into numeric segments.
 * Strips leading "v" prefix if present.
 *
 * "v1.2.3" -> { raw: "v1.2.3", segments: [1, 2, 3] }
 * "0.3"    -> { raw: "0.3",    segments: [0, 3] }
 * "bad"    -> { raw: "bad",    segments: [] }
 */
export function parseVersion(version: string): ParsedVersion {
  const cleaned = String(version).replace(/^v/, "");
  const parts = cleaned.split(".");
  const segments: number[] = [];
  for (const part of parts) {
    const n = parseInt(part, 10);
    if (Number.isNaN(n)) return { raw: version, segments: [] };
    segments.push(n);
  }
  return { raw: version, segments };
}

/**
 * Compare two version strings.
 * Returns 1 if a > b, -1 if a < b, 0 if equal.
 * Missing segments are treated as 0 (e.g. "1.0" == "1.0.0").
 */
export function compareVersions(a: string, b: string): number {
  const pa = parseVersion(a).segments;
  const pb = parseVersion(b).segments;
  const len = Math.max(pa.length, pb.length);
  for (let i = 0; i < len; i++) {
    const av = pa[i] ?? 0;
    const bv = pb[i] ?? 0;
    if (av > bv) return 1;
    if (av < bv) return -1;
  }
  return 0;
}

/**
 * Sort version strings in descending order (newest first).
 * Non-parseable strings sort to the end.
 */
export function sortVersionsDesc(versions: string[]): string[] {
  return [...versions].sort((a, b) => compareVersions(b, a));
}
