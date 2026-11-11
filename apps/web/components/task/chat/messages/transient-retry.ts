import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

export function retryCountdownLabel(milliseconds: number): string {
  const totalSeconds = Math.max(0, Math.ceil(milliseconds / 1_000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

export function parseRetryAt(value: string | undefined): number | undefined {
  const timestamp = parseStrictRfc3339Timestamp(value);
  return timestamp === null ? undefined : Number(timestamp / BigInt(1_000_000));
}
