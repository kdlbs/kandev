import type { TokenUsageMetrics, TokenUsageResponse, UsageAmountKey } from "@/lib/types/http";
import { USAGE_AMOUNT_KEYS } from "@/lib/types/http-usage-history";
import { t } from "@/lib/i18n";

export type UsageIntegerValue = number | string | bigint | null;
export const MAX_SAFE_USAGE_AMOUNT = BigInt(Number.MAX_SAFE_INTEGER);

export function parseUsageInteger(value: UsageIntegerValue | undefined): bigint | null {
  if (typeof value === "bigint") return value >= BigInt(0) ? value : null;
  if (typeof value === "number") {
    return Number.isSafeInteger(value) && value >= 0 ? BigInt(value) : null;
  }
  return typeof value === "string" && /^(0|[1-9]\d*)$/.test(value) ? BigInt(value) : null;
}

export function usageAmount(
  metrics: Pick<TokenUsageMetrics, UsageAmountKey | "amounts_decimal">,
  key: UsageAmountKey,
): bigint | null {
  const exact = metrics.amounts_decimal?.[key];
  return parseUsageInteger(exact === undefined ? metrics[key] : exact);
}

function normalizeMetrics<T extends TokenUsageMetrics>(metrics: T): T {
  const result = { ...metrics };
  for (const key of USAGE_AMOUNT_KEYS) {
    const numeric = metrics[key];
    const exact = metrics.amounts_decimal?.[key];
    if (exact === undefined) {
      if (numeric != null && parseUsageInteger(numeric) === null) invalidAmount();
      continue;
    }
    const amount = parseUsageInteger(exact);
    if (exact !== null && (amount === null || amount > BigInt("9223372036854775807")))
      invalidAmount();
    if (amount !== null && Number.isSafeInteger(numeric) && BigInt(numeric!) !== amount)
      invalidAmount();
    result[key] = amount === null || amount > MAX_SAFE_USAGE_AMOUNT ? null : Number(amount);
  }
  return result;
}

function invalidAmount(): never {
  throw new Error(t("stats:usageAmountTransportInvalid"));
}

export function normalizeTokenUsageResponse(response: TokenUsageResponse): TokenUsageResponse {
  return {
    ...response,
    ...(response.summary && { summary: normalizeMetrics(response.summary) }),
    ...(response.totals && { totals: normalizeMetrics(response.totals) }),
    rows: response.rows.map(normalizeMetrics),
  };
}
