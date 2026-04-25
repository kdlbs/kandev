import { diffLines } from "diff";

export type DiffLineKind = "add" | "remove" | "context";

export type DiffLine = {
  kind: DiffLineKind;
  /** Single line of text without the trailing newline. */
  text: string;
};

/**
 * Line-level diff of two strings in unified order. Each input line maps to one
 * `DiffLine`; identical regions are emitted as `context`, lines present only in
 * `before` as `remove`, and lines present only in `after` as `add`.
 *
 * Trailing newlines are normalized away so a final blank line doesn't show up
 * as a phantom diff entry.
 */
export function lineDiff(before: string, after: string): DiffLine[] {
  const out: DiffLine[] = [];
  for (const part of diffLines(before, after)) {
    const kind = partKind(part);
    // Each part.value can contain multiple lines; split and drop a trailing
    // empty string from the inevitable final "\n".
    const lines = part.value.split("\n");
    if (lines.length > 0 && lines[lines.length - 1] === "") {
      lines.pop();
    }
    for (const text of lines) {
      out.push({ kind, text });
    }
  }
  return out;
}

function partKind(part: { added?: boolean; removed?: boolean }): DiffLineKind {
  if (part.added) return "add";
  if (part.removed) return "remove";
  return "context";
}

/** Counts of added and removed lines in a diff sequence. */
export function diffSummary(lines: DiffLine[]): { added: number; removed: number } {
  let added = 0;
  let removed = 0;
  for (const l of lines) {
    if (l.kind === "add") added++;
    else if (l.kind === "remove") removed++;
  }
  return { added, removed };
}
