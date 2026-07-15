import { defaultFilter } from "cmdk";
import type { CommandItem } from "./types";

const MAX_COMBINED_TERMS_SCORE = 0.99;
const UNORDERED_TERMS_SCORE = 0.98;

export function getCommandSearchTerms(command: CommandItem): string[] {
  return [command.label, ...(command.keywords ?? [])];
}

function tokenize(value: string): string[] {
  return value.toLowerCase().match(/[a-z0-9]+/g) ?? [];
}

function scoreUnorderedTerms(terms: string[], search: string): number {
  const queryTokens = tokenize(search);
  if (queryTokens.length < 2) return 0;

  const termTokens = terms.flatMap(tokenize);
  const allTokensMatch = queryTokens.every((queryToken) =>
    termTokens.some((termToken) => termToken.startsWith(queryToken)),
  );
  return allTokensMatch ? UNORDERED_TERMS_SCORE : 0;
}

export function scoreCommandSearch(_value: string, search: string, searchTerms?: string[]): number {
  const normalizedSearch = search.trim();
  const terms = searchTerms ?? [];
  const bestTermScore = terms.reduce(
    (bestScore, term) => Math.max(bestScore, defaultFilter(term, normalizedSearch, [])),
    0,
  );
  const combinedTermsScore = defaultFilter(terms.join(" "), normalizedSearch, []);
  return Math.max(
    bestTermScore,
    Math.min(combinedTermsScore, MAX_COMBINED_TERMS_SCORE),
    scoreUnorderedTerms(terms, normalizedSearch),
  );
}

function commandSearchScore(command: CommandItem, search: string): number {
  return scoreCommandSearch(command.id, search, getCommandSearchTerms(command));
}

function compareCommandPriority(a: CommandItem, b: CommandItem): number {
  return (a.priority ?? 100) - (b.priority ?? 100);
}

export function sortCommandsForSearch(commands: CommandItem[], search: string): CommandItem[] {
  return [...commands].sort((a, b) => {
    const scoreDifference = commandSearchScore(b, search) - commandSearchScore(a, search);
    return scoreDifference || compareCommandPriority(a, b);
  });
}

export function findFirstMatchingCommand(
  commands: CommandItem[],
  search: string,
): CommandItem | undefined {
  return sortCommandsForSearch(commands, search).find(
    (command) => commandSearchScore(command, search) > 0,
  );
}
