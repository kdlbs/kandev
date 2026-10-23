import type { SourcePosition } from "@/components/editors/source-position";

export function sourceOffsetAtLine(content: string, line: number, column = 1): number {
  const lines = content.split(/\r\n|\r|\n/u);
  const target = Math.max(1, Math.min(lines.length, Math.trunc(line)));
  let start = 0;
  const breaks = content.matchAll(/\r\n|\r|\n/gu);
  for (let current = 1; current < target; current++) {
    const boundary = breaks.next().value;
    if (boundary) start = boundary.index + boundary[0].length;
  }
  return start + Math.max(0, Math.min(lines[target - 1].length, Math.trunc(column) - 1));
}

export function sourceLineAtOffset(content: string, offset: number): number {
  const clamped = Math.max(0, Math.min(content.length, offset));
  return (
    1 +
    Array.from(content.matchAll(/\r\n|\r|\n/gu)).filter(
      (boundary) => boundary.index + boundary[0].length <= clamped,
    ).length
  );
}

export function clampSourcePosition(content: string, position: SourcePosition): SourcePosition {
  const nearestLine = Math.max(
    1,
    Math.min(content.split(/\r\n|\r|\n/u).length, position.nearestLine),
  );
  if (!position.selection) return { nearestLine };
  const clamp = (offset: number) => Math.max(0, Math.min(content.length, offset));
  return {
    nearestLine,
    selection: {
      anchor: clamp(position.selection.anchor),
      active: clamp(position.selection.active),
    },
  };
}
