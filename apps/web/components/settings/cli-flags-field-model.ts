import type { CLIFlag } from "@/lib/types/http";

export function isCuratedFlagMatch(entryFlag: string, curatedFlag: string): boolean {
  const entryTokens = tokenizeCLIFlags(entryFlag);
  const curatedTokens = tokenizeCLIFlags(curatedFlag);
  return (
    entryTokens !== null &&
    curatedTokens !== null &&
    containsTokenSequence(entryTokens, curatedTokens)
  );
}

export function curatedFlagEnabled(
  flags: CLIFlag[],
  flag: string,
  defaultEnabled: boolean,
): boolean {
  const matches = flags.filter((entry) => isCuratedFlagMatch(entry.flag, flag));
  return matches.length > 0 ? matches.some((entry) => entry.enabled) : defaultEnabled;
}

export function setCuratedFlagEnabled(
  flags: CLIFlag[],
  flag: string,
  description: string,
  enabled: boolean,
): CLIFlag[] {
  const curatedTokens = tokenizeCLIFlags(flag);
  if (!curatedTokens?.length) return flags;
  let latestDescription = "";
  const otherFlags: CLIFlag[] = [];
  for (const entry of flags) {
    const tokens = tokenizeCLIFlags(entry.flag);
    if (!tokens || !containsTokenSequence(tokens, curatedTokens)) {
      otherFlags.push(entry);
      continue;
    }
    if (entry.description) latestDescription = entry.description;
    const remaining = removeTokenSequence(tokens, curatedTokens);
    if (remaining.length > 0) {
      otherFlags.push({ ...entry, flag: serializeCLIFlags(remaining) });
    }
  }
  return [...otherFlags, { flag, description: latestDescription || description, enabled }];
}

function containsTokenSequence(tokens: string[], sequence: string[]): boolean {
  if (sequence.length === 0) return false;
  return tokens.some((_, index) =>
    sequence.every((token, offset) => tokens[index + offset] === token),
  );
}

function removeTokenSequence(tokens: string[], sequence: string[]): string[] {
  const remaining: string[] = [];
  for (let index = 0; index < tokens.length; ) {
    if (sequence.every((token, offset) => tokens[index + offset] === token)) {
      index += sequence.length;
      continue;
    }
    remaining.push(tokens[index]);
    index++;
  }
  return remaining;
}

type CLIFlagTokenizer = {
  tokens: string[];
  current: string;
  inToken: boolean;
  quote: "'" | '"' | null;
};

function tokenizeCLIFlags(raw: string): string[] | null {
  const state: CLIFlagTokenizer = { tokens: [], current: "", inToken: false, quote: null };
  for (let index = 0; index < raw.length; index++) {
    const nextIndex = consumeCLIFlagCharacter(state, raw, index);
    if (nextIndex === null) return null;
    index = nextIndex;
  }
  if (state.quote !== null) return null;
  flushCLIFlagToken(state);
  return state.tokens;
}

function consumeCLIFlagCharacter(
  state: CLIFlagTokenizer,
  raw: string,
  index: number,
): number | null {
  const char = raw[index];
  if (state.quote !== null) return consumeQuotedCLIFlagCharacter(state, raw, index);
  if (char === "'" || char === '"') {
    state.quote = char;
    state.inToken = true;
  } else if (char === "\\") {
    return consumeEscapedCLIFlagCharacter(state, raw, index);
  } else if (char === " " || char === "\t" || char === "\n") {
    flushCLIFlagToken(state);
  } else {
    state.current += char;
    state.inToken = true;
  }
  return index;
}

function consumeQuotedCLIFlagCharacter(
  state: CLIFlagTokenizer,
  raw: string,
  index: number,
): number | null {
  const char = raw[index];
  if (char === state.quote) {
    state.quote = null;
  } else if (char === "\\" && state.quote === '"') {
    return consumeEscapedCLIFlagCharacter(state, raw, index);
  } else {
    state.current += char;
    state.inToken = true;
  }
  return index;
}

function consumeEscapedCLIFlagCharacter(
  state: CLIFlagTokenizer,
  raw: string,
  index: number,
): number | null {
  if (index + 1 >= raw.length) return null;
  state.current += raw[index + 1];
  state.inToken = true;
  return index + 1;
}

function flushCLIFlagToken(state: CLIFlagTokenizer): void {
  if (!state.inToken) return;
  state.tokens.push(state.current);
  state.current = "";
  state.inToken = false;
}

function serializeCLIFlags(tokens: string[]): string {
  return tokens.map(serializeCLIFlag).join(" ");
}

function serializeCLIFlag(token: string): string {
  if (token !== "" && !/[\s'"\\]/.test(token)) return token;
  if (!token.includes("'")) return `'${token}'`;
  if (!token.includes('"')) return `"${token}"`;
  return `"${token.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;
}
