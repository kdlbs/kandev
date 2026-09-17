/**
 * Coerces a routine catch-up max to a valid value for display or form state.
 * Any value below 1 (including NaN from an unparsable string) becomes the
 * default of 25, mirroring the backend's own normalization
 * (apps/backend/internal/office/models: NormaliseCatchUpMax).
 */
export function coerceCatchUpMax(value: number | string | undefined): number {
  const numeric = typeof value === "number" ? value : Number(value);
  return Number.isFinite(numeric) && numeric >= 1 ? numeric : 25;
}
