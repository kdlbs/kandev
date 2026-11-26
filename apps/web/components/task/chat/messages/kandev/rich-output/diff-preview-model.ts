type Row = [number] | [number | null, number | null];
type Kind = "header" | "context" | "add" | "remove";
export type PreviewDiff = { lines: string[]; kinds: Kind[]; split: Row[] };

function classify(line: string, oldCount: number, newCount: number): Kind {
  if (oldCount && line.startsWith("-")) return "remove";
  if (newCount && line.startsWith("+")) return "add";
  if (oldCount && newCount && line.startsWith(" ")) return "context";
  return "header";
}

export function parsePreviewDiff(content: string): PreviewDiff | null {
  const header = /^@@ -\d+(?:,(\d+))? \+\d+(?:,(\d+))? @@/;
  const lines = content.split(/\r?\n/);
  if (lines.at(-1) === "") lines.pop();
  const split: Row[] = [];
  const kinds: Kind[] = [];
  let oldCount = 0;
  let newCount = 0;
  let hasHunk = false;
  let removed: number[] = [];
  let added: number[] = [];
  function flushChanges() {
    if (!removed.length && !added.length) return;
    for (let i = 0; i < Math.max(removed.length, added.length); i++) {
      split.push([removed[i] ?? null, added[i] ?? null]);
    }
    removed = [];
    added = [];
  }
  for (const [index, line] of lines.entries()) {
    const kind = classify(line, oldCount, newCount);
    kinds.push(kind);
    if (kind === "remove") {
      oldCount--;
      removed.push(index);
      continue;
    }
    if (kind === "add") {
      newCount--;
      added.push(index);
      continue;
    }
    flushChanges();
    if (kind === "context") {
      oldCount--;
      newCount--;
      split.push([index, index]);
      continue;
    }
    if ((oldCount || newCount) && !line.startsWith("\\")) return null;
    const match = header.exec(line);
    if (match) {
      hasHunk = true;
      oldCount = Number(match[1] ?? 1);
      newCount = Number(match[2] ?? 1);
    }
    split.push([index]);
  }
  flushChanges();
  return hasHunk && oldCount + newCount === 0 ? { lines, split, kinds } : null;
}
