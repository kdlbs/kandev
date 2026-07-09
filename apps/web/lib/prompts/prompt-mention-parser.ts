export type PromptMentionMatch = {
  start: number;
  end: number;
  name: string;
};

export function buildPromptMentionNames(promptNames: string[]) {
  return Array.from(new Set(promptNames.filter(Boolean))).sort(
    (a, b) => b.length - a.length || a.localeCompare(b),
  );
}

export function matchPromptMention(
  content: string,
  index: number,
  promptNames: string[],
): PromptMentionMatch | null {
  if (content[index] !== "@" || !isMentionStart(content, index)) return null;

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
