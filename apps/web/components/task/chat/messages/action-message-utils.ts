import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";

export function readableFailureSummary(message: string): string | null {
  const safe = sanitizeSessionErrorDetails(message);
  return safe !== message || message.length > 240 || message.includes("\n") ? null : safe;
}
