export type PromptReference = {
  id: string;
  name: string;
  content: string;
};

export type PromptReferenceExpansion = {
  name: string;
  content: string;
};

const MAX_PROMPT_REFERENCE_DEPTH = 8;

function isWhitespace(value: string) {
  return value === " " || value === "\n" || value === "\t" || value === "\r";
}

function isMentionNameChar(value: string) {
  return /[A-Za-z0-9_-]/.test(value);
}

function isMentionStart(content: string, index: number) {
  return index === 0 || isWhitespace(content[index - 1]);
}

function buildPromptMap(prompts: PromptReference[]) {
  return new Map(prompts.map((prompt) => [prompt.name, prompt]));
}

function buildPromptNames(prompts: PromptReference[]) {
  return prompts
    .map((prompt) => prompt.name)
    .filter(Boolean)
    .sort((a, b) => b.length - a.length || a.localeCompare(b));
}

type ExpansionState = {
  promptsByName: Map<string, PromptReference>;
  promptNames: string[];
  stack: Set<string>;
  seen: Set<string>;
  expansions: PromptReferenceExpansion[];
};

function matchPromptReference(content: string, index: number, state: ExpansionState) {
  const referenceStart = index + 1;
  for (const name of state.promptNames) {
    if (!content.startsWith(name, referenceStart)) continue;
    const referenceEnd = referenceStart + name.length;
    if (referenceEnd < content.length && isMentionNameChar(content[referenceEnd])) continue;
    return { prompt: state.promptsByName.get(name), referenceEnd };
  }
  return { prompt: undefined, referenceEnd: referenceStart };
}

function collectExpansions(content: string, state: ExpansionState, depth: number): void {
  for (let index = 0; index < content.length; ) {
    if (content[index] !== "@" || !isMentionStart(content, index)) {
      index += 1;
      continue;
    }

    const { prompt, referenceEnd } = matchPromptReference(content, index, state);
    if (!prompt || state.stack.has(prompt.name) || depth >= MAX_PROMPT_REFERENCE_DEPTH) {
      index = referenceEnd;
      continue;
    }

    if (!state.seen.has(prompt.name)) {
      state.seen.add(prompt.name);
      state.expansions.push({ name: prompt.name, content: prompt.content });
      collectExpansions(
        prompt.content,
        // Only stack is copied; seen and expansions are intentionally shared
        // so global dedup and ordered output work across the full DFS tree.
        { ...state, stack: new Set([...state.stack, prompt.name]) },
        depth + 1,
      );
    }
    index = referenceEnd;
  }
}

export function collectPromptReferenceExpansions(
  content: string,
  prompts: PromptReference[],
  currentPromptName?: string,
  initialSeen: Iterable<string> = [],
): PromptReferenceExpansion[] {
  const stack = new Set<string>();
  if (currentPromptName) stack.add(currentPromptName);
  const expansions: PromptReferenceExpansion[] = [];
  collectExpansions(
    content,
    {
      promptsByName: buildPromptMap(prompts),
      promptNames: buildPromptNames(prompts),
      stack,
      seen: new Set(initialSeen),
      expansions,
    },
    0,
  );
  return expansions;
}

export function formatPromptReferenceExpansions(expansions: PromptReferenceExpansion[]) {
  if (expansions.length === 0) return "";
  return [
    "EXPANDED PROMPT REFERENCES: The message above references saved prompts by @name. Use these expansions as hidden context while preserving the original @mentions.",
    ...expansions.map((expansion) => `### @${expansion.name}\n${expansion.content}`),
  ].join("\n\n");
}
