/** Eight injected errors from the November 3, 2026 investigation. The corpus
 * must remain detectable by the reference compiler, default tsrs and fast
 * tsrs, including after a warm incremental pass. */
export type Identified = { id: string; label: string };

export function rename(value: Identified, label: number): Identified {
  return { ...value, label };
}

export function pickId(value: Identified): number {
  return value.id;
}

export function sum(values: readonly number[]): number {
  return values.reduce((total, value) => total + value, 0);
}

export function formatCount(count: string): string {
  return `${count} items`;
}

export function describe(value: Identified): string {
  return value.missing;
}

export const tone: "neutral" | "danger" = "success";

export function assertNever(value: never): void {
  throw new Error(String(value));
}

assertNever("not never");

export function callSum(): number {
  return sum([1, 2], [3]);
}

export const wrongShape: Identified = { id: "one" };

export const excessProperty: Identified = { id: "two", label: "Two", extra: true };
