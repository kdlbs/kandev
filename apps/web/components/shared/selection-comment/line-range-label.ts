import { t } from "@/lib/i18n";

/** Context label for a code or Markdown range comment. */
export function lineRangeLabel(start: number, end: number): string {
  return start === end
    ? t("task:lineNumber", { line: start })
    : t("task:lineRange", { start, end });
}
