import type { UsageTurn } from "@/lib/types/conversation-usage";

const integerFormatter = new Intl.NumberFormat();

export function formatUsageTokens(value: number): string {
  return integerFormatter.format(value);
}

/** Format ledger subcents (1/100 of a cent) without converting int64 to float. */
export function formatUsageCost(subcents?: string): string | null {
  if (subcents === undefined || !/^-?\d+$/.test(subcents)) return null;
  let amount: bigint;
  try {
    amount = BigInt(subcents);
  } catch {
    return null;
  }
  const zero = BigInt(0);
  const negative = amount < zero;
  const absolute = negative ? -amount : amount;
  const cents = (absolute + BigInt(50)) / BigInt(100);
  const dollars = cents / BigInt(100);
  const remainder = (cents % BigInt(100)).toString().padStart(2, "0");
  return `${negative ? "-$" : "$"}${dollars.toLocaleString()}.${remainder}`;
}

export function latestUsageTurn(turns: UsageTurn[]): UsageTurn | undefined {
  return turns[0];
}

export function usageDetailForTurn<T extends { turnId: string; turn: UsageTurn | null }>(
  latestTurn: UsageTurn | null,
  detail: T | null,
): T | null {
  if (!latestTurn || !detail || detail.turnId !== latestTurn.turn_id) return null;
  if (detail.turn && detail.turn.turn_id !== latestTurn.turn_id) return null;
  return detail;
}
