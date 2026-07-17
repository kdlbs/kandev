import type { ContextWindowEntry } from "./types";

export function parseContextWindowEntry(
  value: unknown,
  timestamp?: string,
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
    source,
    timestamp: timestamp ?? (contextWindow.timestamp as string) ?? undefined,
  };
}
