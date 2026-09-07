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
const MAX_PROMPT_REFERENCE_EXPANSIONS = 128;
const MAX_PROMPT_EXPANSION_BYTES = 4 * 1024 * 1024;
const KANDEV_SYSTEM_TAG_END = "</kandev-system>";
const textEncoder = new TextEncoder();

function buildPromptMap(prompts: PromptReference[]) {
  return new Map(prompts.map((prompt) => [prompt.name, prompt]));
}

type ExpansionState = {
  promptsByName: Map<string, PromptReference>;
  promptNames: string[];
  stack: Set<string>;
  seen: Set<string>;
  expansions: PromptReferenceExpansion[];
  budget: {
    bytes: number;
    exceeded: boolean;
  };
};

function collectExpansions(content: string, state: ExpansionState, depth: number): void {
  for (let index = 0; index < content.length; ) {
    if (state.budget.exceeded) return;
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
      const expansionBytes =
        textEncoder.encode(prompt.name).byteLength + textEncoder.encode(prompt.content).byteLength;
      if (
        state.expansions.length >= MAX_PROMPT_REFERENCE_EXPANSIONS ||
        state.budget.bytes + expansionBytes > MAX_PROMPT_EXPANSION_BYTES
      ) {
        state.budget.exceeded = true;
        return;
      }
      state.budget.bytes += expansionBytes;
      state.seen.add(prompt.name);
      state.expansions.push({ name: prompt.name, content: prompt.content });
      collectExpansions(
        prompt.content,
        {
          ...state,
          stack: new Set([...state.stack, prompt.name]),
        },
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
  const state: ExpansionState = {
    promptsByName: buildPromptMap(prompts),
    promptNames: buildPromptMentionNames(prompts.map((prompt) => prompt.name)),
    stack,
    seen: new Set(initialSeen),
    expansions,
    budget: { bytes: 0, exceeded: false },
  };
  collectExpansions(content, state, 0);
  return state.budget.exceeded ? [] : state.expansions;
}

export function formatPromptReferenceExpansions(expansions: PromptReferenceExpansion[]) {
  if (expansions.length === 0) return "";
  return [
    "EXPANDED PROMPT REFERENCES: The message above references saved prompts by @name. Use these expansions as hidden context while preserving the original @mentions.",
    ...expansions.map(
      (expansion) =>
        `### @${sanitizePromptReferenceSystemText(expansion.name)}\n${sanitizePromptReferenceSystemText(expansion.content)}`,
    ),
  ].join("\n\n");
}

export function sanitizePromptReferenceSystemText(value: string) {
  let sanitized = value;
  while (sanitized.includes(KANDEV_SYSTEM_TAG_END)) {
    sanitized = sanitized.replaceAll(KANDEV_SYSTEM_TAG_END, "");
  }
  return sanitized;
}
