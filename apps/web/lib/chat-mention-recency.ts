import type { MentionItem } from "@/hooks/use-inline-mention";

export const CHAT_MENTION_RECENCY_STORAGE_KEY = "kandev.chatMentionRecency.v1";
export const MAX_CHAT_MENTION_RECENT_ENTRIES = 50;

type ChatMentionRecentKind = "task" | "prompt" | "file";

export type ChatMentionRecentEntry = {
  kind: ChatMentionRecentKind;
  id: string;
  workspaceId?: string;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function nonEmptyString(value: unknown): string | null {
  return typeof value === "string" && value.length > 0 ? value : null;
}

function isRecentKind(value: unknown): value is ChatMentionRecentKind {
  return value === "task" || value === "prompt" || value === "file";
}

function recentEntryKey(entry: ChatMentionRecentEntry): string {
  return `${entry.kind}:${entry.workspaceId ?? ""}:${entry.id}`;
}

function normalizeRecentEntry(value: unknown): ChatMentionRecentEntry | null {
  if (!isRecord(value) || !isRecentKind(value.kind)) return null;
  const id = nonEmptyString(value.id);
  if (!id) return null;

  if (value.kind === "file") {
    const workspaceId = nonEmptyString(value.workspaceId);
    if (!workspaceId) return null;
    return { kind: value.kind, id, workspaceId };
  }

  return { kind: value.kind, id };
}

export function normalizeChatMentionRecentEntries(value: unknown): ChatMentionRecentEntry[] {
  if (!Array.isArray(value)) return [];
  const normalized: ChatMentionRecentEntry[] = [];
  const seen = new Set<string>();

  for (const candidate of value) {
    const entry = normalizeRecentEntry(candidate);
    if (!entry) continue;
    const key = recentEntryKey(entry);
    if (seen.has(key)) continue;
    seen.add(key);
    normalized.push(entry);
    if (normalized.length === MAX_CHAT_MENTION_RECENT_ENTRIES) break;
  }

  return normalized;
}

function getLocalStorage(): Storage | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

export function getChatMentionRecentEntries(): ChatMentionRecentEntry[] {
  const storage = getLocalStorage();
  if (!storage) return [];

  try {
    const raw = storage.getItem(CHAT_MENTION_RECENCY_STORAGE_KEY);
    return raw ? normalizeChatMentionRecentEntries(JSON.parse(raw)) : [];
  } catch {
    return [];
  }
}

export function mentionItemToRecentEntry(
  item: MentionItem,
  workspaceId: string | null | undefined,
): ChatMentionRecentEntry | null {
  if (item.kind === "plan") return null;

  if (item.kind === "file") {
    const scopedWorkspaceId = nonEmptyString(workspaceId);
    const id = nonEmptyString(item.id);
    if (!scopedWorkspaceId || !id) return null;
    return { kind: item.kind, id, workspaceId: scopedWorkspaceId };
  }

  const id = nonEmptyString(item.kind === "task" ? (item.task?.taskId ?? item.id) : item.id);
  return id ? { kind: item.kind, id } : null;
}

function writeRecentEntries(entries: ChatMentionRecentEntry[]): void {
  const storage = getLocalStorage();
  if (!storage) return;
  try {
    storage.setItem(CHAT_MENTION_RECENCY_STORAGE_KEY, JSON.stringify(entries));
  } catch {
    // Ignore private browsing, blocked storage, and quota failures.
  }
}

export function recordChatMentionSelection(
  item: MentionItem,
  workspaceId: string | null | undefined,
): ChatMentionRecentEntry[] {
  const entry = mentionItemToRecentEntry(item, workspaceId);
  if (!entry) return getChatMentionRecentEntries();

  const current = getChatMentionRecentEntries().filter(
    (candidate) => recentEntryKey(candidate) !== recentEntryKey(entry),
  );
  const updated = [entry, ...current].slice(0, MAX_CHAT_MENTION_RECENT_ENTRIES);
  writeRecentEntries(updated);
  return updated;
}

type ScoredMentionItem = {
  item: MentionItem;
  score: number;
  baselineIndex: number;
};

function textMatchScore(label: string, query: string): number {
  if (!query) return 0;
  const lowerLabel = label.toLowerCase();
  if (lowerLabel.startsWith(query)) return 100;
  if (lowerLabel.split(/[\s\-_/]/).some((word) => word.startsWith(query))) return 50;
  if (lowerLabel.includes(query)) return 25;
  return 0;
}

function buildBaseline(items: MentionItem[], query: string): ScoredMentionItem[] {
  const lowerQuery = query.toLowerCase();
  return items
    .map((item, baselineIndex) => ({
      item,
      score: textMatchScore(item.label, lowerQuery),
      baselineIndex,
    }))
    .filter(({ score }) => !query || score > 0)
    .sort((a, b) => b.score - a.score || a.baselineIndex - b.baselineIndex);
}

export function rankMentionItems(
  items: MentionItem[],
  query: string,
  workspaceId: string | null | undefined,
  recentEntries: readonly ChatMentionRecentEntry[] = getChatMentionRecentEntries(),
): MentionItem[] {
  const baseline = buildBaseline(items, query);
  const normalizedRecentEntries = normalizeChatMentionRecentEntries(recentEntries);
  if (normalizedRecentEntries.length === 0) return baseline.map(({ item }) => item);

  const recentPositions = new Map(
    normalizedRecentEntries.map((entry, index) => [recentEntryKey(entry), index]),
  );
  const planIndex = baseline.findIndex(({ item }) => item.kind === "plan");
  const plan = planIndex >= 0 ? baseline[planIndex] : undefined;
  const candidates = baseline.filter(({ item }) => item.kind !== "plan");

  candidates.sort((a, b) => {
    const aEntry = mentionItemToRecentEntry(a.item, workspaceId);
    const bEntry = mentionItemToRecentEntry(b.item, workspaceId);
    const aRecent = aEntry ? recentPositions.get(recentEntryKey(aEntry)) : undefined;
    const bRecent = bEntry ? recentPositions.get(recentEntryKey(bEntry)) : undefined;
    const aKnown = aRecent !== undefined;
    const bKnown = bRecent !== undefined;

    if (aKnown !== bKnown) return aKnown ? -1 : 1;
    if (aKnown && bKnown && aRecent !== bRecent) return aRecent! - bRecent!;
    return b.score - a.score || a.baselineIndex - b.baselineIndex;
  });

  const ranked = candidates.map(({ item }) => item);
  if (!plan) return ranked;
  ranked.splice(Math.min(planIndex, ranked.length), 0, plan.item);
  return ranked;
}
