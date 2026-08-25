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
  preferredCommandId?: string,
): CommandItem | undefined {
  const preferredCommand = preferredCommandId
    ? commands.find((command) => command.id === preferredCommandId)
    : undefined;
  if (preferredCommand && commandSearchScore(preferredCommand, search) > 0) {
    return preferredCommand;
  }
  return sortCommandsForSearch(commands, search).find(
    (command) => commandSearchScore(command, search) > 0,
  );
}

/**
 * Picks the highlighted row for content search, which publishes its results
 * incrementally while later retry attempts are still running. A selection the
 * user moved is kept as long as it still points at a row: resetting to the
 * first result whenever a late repository appends would pull focus back and
 * open a different file than the highlighted one.
 */
export function selectContentSearchResult(resultValues: string[], preferredValue?: string): string {
  if (preferredValue && resultValues.includes(preferredValue)) return preferredValue;
  return resultValues[0] ?? "";
}

export type CommandSearchSelectionOptions = {
  commands: CommandItem[];
  search: string;
  /** Values of the task rows rendered alongside the commands, in list order. */
  taskResultValues: string[];
  preferredValue?: string;
  /**
   * True while matching commands render above the task rows. The palette only
   * leads with tasks when nothing has been typed: once there is a query, an
   * exact command match outranks a fuzzy task match.
   */
  commandsLeadResults: boolean;
};

/**
 * Picks the highlighted row for the commands scope, which mixes commands with
 * task rows. The default highlight follows the rendered order so Enter always
 * runs the row the user is looking at, and a selection the user moved survives
 * a command re-registration as long as its row is still rendered.
 */
export function selectCommandSearchResult(options: CommandSearchSelectionOptions): string {
  const { commands, search, taskResultValues, preferredValue, commandsLeadResults } = options;
  const normalizedSearch = search.trim();
  if (preferredValue) {
    if (taskResultValues.includes(preferredValue)) return preferredValue;
    const preferredCommand = commands.find((command) => command.id === preferredValue);
    const preferredCommandStillVisible =
      preferredCommand &&
      (!normalizedSearch ||
        findFirstMatchingCommand(commands, normalizedSearch, preferredValue)?.id ===
          preferredValue);
    if (preferredCommandStillVisible) return preferredValue;
  }

  const firstCommand = normalizedSearch
    ? (findFirstMatchingCommand(commands, normalizedSearch)?.id ?? "")
    : "";
  const firstTask = taskResultValues[0] ?? "";
  return commandsLeadResults ? firstCommand || firstTask : firstTask || firstCommand;
}
