/**
 * Converts between API instants (ISO 8601, UTC) and the value of a
 * `datetime-local` input, which is wall-clock time in the browser's zone.
 */
export function toLocalInputValue(iso: string | undefined): string {
  if (!iso) return "";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  const pad = (value: number) => String(value).padStart(2, "0");
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
    `T${pad(date.getHours())}:${pad(date.getMinutes())}`
  );
}

export function fromLocalInputValue(value: string): string | null {
  if (!value) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date.toISOString();
}

/** The browser's IANA zone, so a monthly reset keeps its local clock time. */
export function browserTimeZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

/** The provider prefix of a provider-qualified model ID, lowercased. */
export function providerOfModel(model: string | undefined): string {
  const trimmed = (model ?? "").trim();
  const slash = trimmed.indexOf("/");
  return slash > 0 ? trimmed.slice(0, slash).toLowerCase() : "";
}
