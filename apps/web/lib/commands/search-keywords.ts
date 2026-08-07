import type { TFunction } from "i18next";

/**
 * Search keywords are stored as one comma-separated catalog value so a
 * translator can localize the whole set in one entry. They are matched, never
 * displayed; the palette itself selects commands by `id` (see
 * `command-panel-footer.tsx`), so no behavior keys off this copy.
 */
export function searchKeywords(t: TFunction, key: string): string[] {
  return t(key)
    .split(",")
    .map((keyword) => keyword.trim())
    .filter(Boolean);
}
