import type { ResolvedSettingsDiscoveryItem } from "./types";

type ScoredItem = {
  item: ResolvedSettingsDiscoveryItem;
  score: number;
};

function normalize(value: string): string {
  return value
    .normalize("NFKD")
    .replace(/\p{M}+/gu, "")
    .toLocaleLowerCase()
    .trim()
    .replace(/\s+/g, " ");
}

function tokenize(value: string): string[] {
  return normalize(value).match(/[\p{L}\p{N}]+/gu) ?? [];
}

function containsToken(values: string[], token: string): boolean {
  return values.some((value) => value.includes(token));
}

function scoreItem(item: ResolvedSettingsDiscoveryItem, query: string): number | null {
  const normalizedLabel = normalize(item.label);
  const normalizedAliases = item.aliases.map(normalize);
  const directValues = [normalizedLabel, ...normalizedAliases];
  const contextValues = item.breadcrumb.map(normalize);
  const tokens = tokenize(query);
  if (tokens.length === 0) return null;
  if (!tokens.every((token) => containsToken([...directValues, ...contextValues], token))) {
    return null;
  }
  if (!tokens.some((token) => containsToken(directValues, token))) return null;

  const normalizedQuery = normalize(query);
  if (normalizedLabel === normalizedQuery) return 0;
  if (normalizedAliases.includes(normalizedQuery)) return 10;
  if (normalizedLabel.startsWith(normalizedQuery)) return 20;
  if (normalizedLabel.includes(normalizedQuery)) return 25;
  if (normalizedAliases.some((alias) => alias.startsWith(normalizedQuery))) return 30;
  return 40;
}

export function searchSettingsDiscovery(
  items: ResolvedSettingsDiscoveryItem[],
  query: string,
): ResolvedSettingsDiscoveryItem[] {
  return items
    .map((item): ScoredItem | null => {
      const score = scoreItem(item, query);
      return score === null ? null : { item, score };
    })
    .filter((entry): entry is ScoredItem => entry !== null)
    .sort((a, b) => a.score - b.score || a.item.order - b.item.order)
    .map(({ item }) => item);
}
