export type PromptReference = {
  id: string;
  name: string;
  content: string;
};

export type PromptReferenceExpansion = {
  name: string;
  content: string;
};

import { buildPromptMentionNames, matchPromptMention } from "./prompt-mention-parser";

const MAX_PROMPT_REFERENCE_DEPTH = 8;
const KANDEV_SYSTEM_TAG_END = "</kandev-system>";

function buildPromptMap(prompts: PromptReference[]) {
  return new Map(prompts.map((prompt) => [prompt.name, prompt]));
}

type ExpansionState = {
  promptsByName: Map<string, PromptReference>;
  promptNames: string[];
  stack: Set<string>;
  seen: Set<string>;
  expansions: PromptReferenceExpansion[];
};

function collectExpansions(content: string, state: ExpansionState, depth: number): void {
  for (let index = 0; index < content.length; ) {
    const match = matchPromptMention(content, index, state.promptNames);
    if (!match) {
      index += 1;
      continue;
    }

    const prompt = state.promptsByName.get(match.name);
    if (!prompt || state.stack.has(prompt.name) || depth >= MAX_PROMPT_REFERENCE_DEPTH) {
      index = match.end;
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
    index = match.end;
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
      promptNames: buildPromptMentionNames(prompts.map((prompt) => prompt.name)),
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
    ...expansions.map(
      (expansion) =>
        `### @${stripKandevSystemTagEnd(expansion.name)}\n${stripKandevSystemTagEnd(expansion.content)}`,
    ),
  ].join("\n\n");
}

function stripKandevSystemTagEnd(value: string) {
  return value.replaceAll(KANDEV_SYSTEM_TAG_END, "");
}
