// Mirror of the Go `readselector` package
// (apps/backend/internal/common/readselector). OMP's read tool embeds a
// line/range/mode selector in the path ("foo.go:43-94") and can reference
// several comma-joined files in a single read ("a.yaml:1-80,b.yaml:1-80"). The
// backend keeps a multi-file read's path raw; this splits it into one openable
// entry per file (selector stripped, line range parsed) so the chat can render a
// separate link per file. Keep in sync with the Go implementation and its tests.

export type ReadFileRef = {
  /** Bare, openable file path with any read selector stripped. */
  path: string;
  /** 1-based first line referenced by the selector, or 0 when none. */
  startLine: number;
  /** Contiguous line span for a closed range ("N-M" / "N+K"), else 0. */
  lineCount: number;
};

type LineSpec = { startLine: number; lineCount: number };

// atoi mirrors Go strconv.Atoi for our needs: an optionally-signed decimal
// integer only (no hex, exponent, or surrounding whitespace).
function atoi(s: string): number | null {
  if (!/^-?\d+$/.test(s)) return null;
  return Number(s);
}

// parseLineSpec parses a single line spec: "N", "N-", "N-M", or "N+K".
function parseLineSpec(seg: string): LineSpec | null {
  const plus = seg.indexOf("+");
  if (plus >= 0) {
    const start = atoi(seg.slice(0, plus));
    const count = atoi(seg.slice(plus + 1));
    if (start === null || count === null || start <= 0 || count < 0) return null;
    return { startLine: start, lineCount: count };
  }
  const dash = seg.indexOf("-");
  if (dash >= 0) {
    const start = atoi(seg.slice(0, dash));
    if (start === null || start <= 0) return null;
    const rest = seg.slice(dash + 1);
    if (rest === "") return { startLine: start, lineCount: 0 }; // "N-" open-ended
    const end = atoi(rest);
    if (end === null || end < start) return null;
    return { startLine: start, lineCount: end - start + 1 };
  }
  const start = atoi(seg);
  if (start === null || start <= 0) return null;
  return { startLine: start, lineCount: 0 };
}

// parseLineSpecList parses a comma-separated list ("5-16,960-973"); the first
// spec drives the result.
function parseLineSpecList(part: string): LineSpec | null {
  const segs = part.split(",");
  let first: LineSpec | null = null;
  for (let i = 0; i < segs.length; i++) {
    const spec = parseLineSpec(segs[i] ?? "");
    if (!spec) return null;
    if (i === 0) first = spec;
  }
  return first;
}

// parseReadSelector validates the selector tail (everything after the file's
// first ':'). Parts joined by ':' accept combos such as "2-4:raw"; the first
// line-spec encountered drives the range. Returns a zero range (still valid) for
// mode-only selectors like "raw".
function parseReadSelector(suffix: string): LineSpec | null {
  let result: LineSpec = { startLine: 0, lineCount: 0 };
  let gotLine = false;
  for (const part of suffix.split(":")) {
    if (part === "raw" || part === "conflicts") continue;
    const spec = parseLineSpecList(part);
    if (!spec) return null;
    if (!gotLine) {
      result = spec;
      gotLine = true;
    }
  }
  return result;
}

// splitOne mirrors Go readselector.Split for a single file reference. A colon in
// the file's final segment that isn't a Windows drive letter ("C:\...") begins
// the selector.
function splitOne(raw: string): ReadFileRef {
  const unchanged: ReadFileRef = { path: raw, startLine: 0, lineCount: 0 };
  let lastSep = raw.lastIndexOf("/");
  const back = raw.lastIndexOf("\\");
  if (back > lastSep) lastSep = back;
  const rel = raw.slice(lastSep + 1).indexOf(":");
  if (rel < 0) return unchanged;
  const colon = lastSep + 1 + rel;
  const isWindowsDrivePrefix = colon === 1 && raw.length >= 2 && /^[A-Za-z]$/.test(raw[0] ?? "");
  if (lastSep < 0 && isWindowsDrivePrefix) return unchanged;
  const base = raw.slice(0, colon);
  const suffix = raw.slice(colon + 1);
  if (base === "" || suffix === "") return unchanged;
  const parsed = parseReadSelector(suffix);
  if (!parsed) return unchanged;
  return { path: base, startLine: parsed.startLine, lineCount: parsed.lineCount };
}

/**
 * Split an omp read path that may reference several comma-joined files into one
 * {@link ReadFileRef} per file. A single file (with or without a multi-range
 * selector) returns exactly one entry identical to the Go `Split`; multiple
 * files are only reported when every segment yields a real file path (one with a
 * path separator or a filename extension), so a comma living inside a directory
 * name is preserved as a single entry.
 */
export function splitReadFiles(raw: string): ReadFileRef[] {
  const parts = raw.split(",");
  const files: ReadFileRef[] = [];
  parts.forEach((part, i) => {
    // A bare line-spec list or mode keyword is an extra range of the preceding
    // file, not a new file.
    const isContinuationRange =
      part === "raw" || part === "conflicts" || parseLineSpecList(part) !== null;
    if (i > 0 && isContinuationRange) return;
    files.push(splitOne(part));
  });
  const allFileish = files.every((f) => /[/\\]/.test(f.path) || f.path.includes("."));
  if (files.length > 1 && allFileish) return files;
  return [splitOne(raw)];
}
