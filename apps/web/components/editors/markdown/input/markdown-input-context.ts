/** Native controls normalize CRLF while the editor's source stays byte preserving. */
export function projectMarkdownInput(source: string) {
  const boundaries = [0];
  const nativeBoundaries = [0];
  let value = "";
  for (let i = 0; i < source.length; i++) {
    nativeBoundaries[i] = value.length;
    if (source[i] === "\r" && source[i + 1] === "\n") {
      nativeBoundaries[++i] = value.length;
    }
    value += source[i] === "\r" ? "\n" : source[i];
    boundaries.push(i + 1);
  }
  nativeBoundaries[source.length] = value.length;
  return {
    value,
    toSource: (offset: number) => boundaries[Math.max(0, Math.min(value.length, offset))],
    toNative: (offset: number) => nativeBoundaries[Math.max(0, Math.min(source.length, offset))],
  };
}

export class MarkdownInputContext extends EventTarget {
  constructor(public text: string) {
    super();
  }
  updateText(start: number, end: number, text: string): void {
    this.text = this.text.slice(0, start) + text + this.text.slice(end);
  }
  updateSelection(_start: number, _end: number): void {}
}

export type NativeReplacementRange = { start: number; end: number };

export function nativeMarkdownReplacement(
  source: string,
  value: string,
  hint?: NativeReplacementRange,
) {
  const projection = projectMarkdownInput(source);
  let start = 0;
  while (
    start < projection.value.length &&
    start < value.length &&
    projection.value[start] === value[start]
  )
    start++;
  let end = projection.value.length;
  let nativeEnd = value.length;
  while (end > start && nativeEnd > start && projection.value[end - 1] === value[nativeEnd - 1]) {
    end--;
    nativeEnd--;
  }
  if (
    hint &&
    value.startsWith(projection.value.slice(0, hint.start)) &&
    value.endsWith(projection.value.slice(hint.end)) &&
    value.length >= hint.start + projection.value.length - hint.end
  ) {
    start = hint.start;
    end = hint.end;
    nativeEnd = value.length - (projection.value.length - end);
  }
  const sourceStart = projection.toSource(start);
  const style = markdownLineEnding(source, sourceStart);
  return {
    start: sourceStart,
    end: projection.toSource(end),
    text: value.slice(start, nativeEnd).replace(/\n/g, style),
  };
}

function markdownLineEnding(source: string, offset: number): string {
  const lineEnd = source.indexOf("\n", offset);
  if (lineEnd >= 0) return source[lineEnd - 1] === "\r" ? "\r\n" : "\n";
  const endings = source.match(/\r\n|\n|\r/g) ?? [];
  const crlf = endings.filter((ending) => ending === "\r\n").length;
  return crlf > endings.length / 2 ? "\r\n" : "\n";
}
