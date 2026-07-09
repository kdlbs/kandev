export type PromptMentionSegment =
  | { kind: "text"; value: string }
  | { kind: "prompt"; value: string; name: string };

export function splitPromptMentionSegments(
  content: string,
  promptNames: string[],
): PromptMentionSegment[] {
  const names = buildPromptNames(promptNames);
  if (content.length === 0 || names.length === 0) return [{ kind: "text", value: content }];

  const segments: PromptMentionSegment[] = [];
  let lastIndex = 0;
  for (let index = 0; index < content.length; ) {
    if (content[index] !== "@" || !isMentionStart(content, index)) {
      index += 1;
      continue;
    }

    const match = matchPromptMention(content, index, names);
    if (!match) {
      index += 1;
      continue;
    }

    if (match.start > lastIndex) {
      segments.push({ kind: "text", value: content.slice(lastIndex, match.start) });
    }
    segments.push({
      kind: "prompt",
      value: content.slice(match.start, match.end),
      name: match.name,
    });
    index = match.end;
    lastIndex = match.end;
  }

  if (lastIndex < content.length) {
    segments.push({ kind: "text", value: content.slice(lastIndex) });
  }
  return segments.length > 0 ? segments : [{ kind: "text", value: content }];
}

function buildPromptNames(promptNames: string[]) {
  return Array.from(new Set(promptNames.filter(Boolean))).sort(
    (a, b) => b.length - a.length || a.localeCompare(b),
  );
}

function matchPromptMention(content: string, index: number, promptNames: string[]) {
  const referenceStart = index + 1;
  for (const name of promptNames) {
    if (!content.startsWith(name, referenceStart)) continue;
    const referenceEnd = referenceStart + name.length;
    if (referenceEnd < content.length && isMentionNameChar(content[referenceEnd])) continue;
    return { start: index, end: referenceEnd, name };
  }
  return null;
}

function isMentionStart(content: string, index: number) {
  return index === 0 || isWhitespace(content[index - 1]);
}

function isWhitespace(value: string) {
  return value === " " || value === "\n" || value === "\t" || value === "\r";
}

function isMentionNameChar(value: string) {
  return /[A-Za-z0-9_-]/.test(value);
}
