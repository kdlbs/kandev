import type { ContextWindowEntry } from "./types";

function normalizeCompactionCount(value: unknown): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return 0;
  return Math.max(0, Math.trunc(value));
}

export function parseContextWindowEntry(
  value: unknown,
  timestamp?: string,
  compactionCount?: unknown,
): ContextWindowEntry | null {
  if (!value || typeof value !== "object") return null;

  const contextWindow = value as Record<string, unknown>;
  const source =
    contextWindow.source === "acp" || contextWindow.source === "api"
      ? contextWindow.source
      : undefined;

  return {
    size: (contextWindow.size as number) ?? 0,
    used: (contextWindow.used as number) ?? 0,
    remaining: (contextWindow.remaining as number) ?? 0,
    efficiency: (contextWindow.efficiency as number) ?? 0,
    compactionCount: normalizeCompactionCount(compactionCount),
    source,
    timestamp: timestamp ?? (contextWindow.timestamp as string | undefined),
  };
}
