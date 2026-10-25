export type PromptMentionMatch = {
  start: number;
  end: number;
  name: string;
};

const MAX_PROMPT_MENTION_NAMES = 2000;
const MAX_PROMPT_MENTION_NAME_LENGTH = 512;
const MAX_PROMPT_MENTION_NAME_BYTES = 200_000;

export function buildPromptMentionNames(promptNames: string[]) {
  const names = Array.from(
    new Set(
      promptNames.filter((name) => Boolean(name) && name.length <= MAX_PROMPT_MENTION_NAME_LENGTH),
    ),
  ).sort((a, b) => b.length - a.length || a.localeCompare(b));
  const accepted: string[] = [];
  let totalLength = 0;
  for (const name of names) {
    if (accepted.length >= MAX_PROMPT_MENTION_NAMES) break;
    if (totalLength + name.length > MAX_PROMPT_MENTION_NAME_BYTES) continue;
    accepted.push(name);
    totalLength += name.length;
  }
  return accepted;
}
type PromptNameTrieNode = {
  children: Map<string, PromptNameTrieNode>;
  name?: string;
};

const promptNamePrefixCache = new WeakMap<readonly string[], PromptNameTrieNode>();

function getPromptNamePrefixIndex(promptNames: string[]) {
  const cached = promptNamePrefixCache.get(promptNames);
  if (cached) return cached;
  const root: PromptNameTrieNode = { children: new Map() };
  for (const name of promptNames) {
    let node = root;
    for (const character of name) {
      let child = node.children.get(character);
      if (!child) {
        child = { children: new Map() };
        node.children.set(character, child);
      }
      node = child;
    }
    node.name = name;
  }
  promptNamePrefixCache.set(promptNames, root);
  return root;
}

/**
 * Match using names ordered by buildPromptMentionNames so longer names win
 * over shorter prefixes.
 */
export function matchPromptMention(
  content: string,
  index: number,
  promptNames: string[],
): PromptMentionMatch | null {
  if (content[index] !== "@" || !isMentionStart(content, index)) return null;

  const referenceStart = index + 1;
  let node = getPromptNamePrefixIndex(promptNames);
  let bestMatch: PromptMentionMatch | null = null;
  for (let cursor = referenceStart; cursor < content.length; ) {
    const codePoint = content.codePointAt(cursor);
    if (codePoint === undefined) break;
    const character = String.fromCodePoint(codePoint);
    const child = node.children.get(character);
    if (!child) break;
    node = child;
    cursor += character.length;
    if (node.name && (cursor >= content.length || !isMentionNameCharAt(content, cursor))) {
      bestMatch = { start: index, end: cursor, name: node.name };
    }
  }
  return bestMatch;
}

function isMentionNameCharAt(content: string, index: number) {
  const codePoint = content.codePointAt(index);
  return codePoint !== undefined && /[\p{L}\p{M}\p{N}_-]/u.test(String.fromCodePoint(codePoint));
}

function isMentionStart(content: string, index: number) {
  return index === 0 || isWhitespace(content[index - 1]);
}

function isWhitespace(value: string) {
  return value === " " || value === "\n" || value === "\t" || value === "\r";
}
