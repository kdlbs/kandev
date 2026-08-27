"use client";

import { Badge } from "@kandev/ui/badge";
import { SEVERITY_LABEL_KEYS } from "@/lib/review/format";
import type { ReviewSeverity } from "@/lib/types/review";
import { t } from "@/lib/i18n";

/**
 * Severity styling. `blocker` and `major` carry destructive weight because they
 * assert something is wrong; `minor` and `nit` stay muted so a long tail of
 * small notes cannot visually drown a real defect.
 */
const SEVERITY_CLASS: Record<ReviewSeverity, string> = {
  blocker: "border-destructive/40 bg-destructive/15 text-destructive",
  major: "border-amber-500/40 bg-amber-500/15 text-amber-700 dark:text-amber-400",
  minor: "border-border bg-muted text-muted-foreground",
  nit: "border-border bg-muted text-muted-foreground",
};

export function ReviewFindingSeverityBadge({ severity }: { severity: ReviewSeverity }) {
  return (
    <Badge
      variant="outline"
      className={`px-1.5 py-0 text-[10px] font-semibold uppercase tracking-wide ${
        SEVERITY_CLASS[severity] ?? SEVERITY_CLASS.minor
      }`}
      data-testid={`review-finding-severity-${severity}`}
    >
      {t(SEVERITY_LABEL_KEYS[severity] ?? severity)}
    </Badge>
  );
}
