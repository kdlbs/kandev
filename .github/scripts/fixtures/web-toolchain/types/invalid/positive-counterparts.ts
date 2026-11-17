/** Positive counterparts that share the invalid project's compilation unit. */
import type { Identified } from "./injected-errors";

export function renameValid(value: Identified, label: string): Identified {
  return { ...value, label };
}

export function pickIdValid(value: Identified): string {
  return value.id;
}

export const correctShape: Identified = { id: "one", label: "One" };
