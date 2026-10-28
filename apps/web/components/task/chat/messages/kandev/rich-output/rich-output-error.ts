const SUMMARY_MAX_LENGTH = 160;

export function getRichOutputErrorText(result: unknown, fallback: string): string {
  if (typeof result === "string" && result.trim()) return result;
  if (typeof result !== "object" || result === null || Array.isArray(result)) return fallback;

  const record = result as Record<string, unknown>;
  for (const field of ["message", "error"]) {
    const value = record[field];
    if (typeof value === "string" && value.trim()) return value;
  }
  return fallback;
}

export function getRichOutputErrorSummary(diagnostic: string): string {
  const firstLine = diagnostic.split(/\r?\n/).find((line) => line.trim());
  const summary = firstLine?.trim() ?? "";
  const characters = Array.from(summary);
  if (characters.length <= SUMMARY_MAX_LENGTH) return summary;
  return `${characters.slice(0, SUMMARY_MAX_LENGTH - 1).join("")}…`;
}
