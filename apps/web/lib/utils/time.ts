import { formatRelative } from "@/lib/i18n/formats";

/**
 * @deprecated Use `formatRelative` from `@/lib/i18n/formats` directly. Kept as a
 * thin, behavior-compatible alias so existing call sites keep working while
 * routing through the locale-aware relative-time formatter. Under `en` the
 * output is unchanged ("just now", "{n}m ago", "{n}h ago", "{n}d ago").
 */
export function timeAgo(dateStr: string): string {
  return formatRelative(dateStr);
}
