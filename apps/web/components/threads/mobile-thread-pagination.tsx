"use client";

import { cn } from "@kandev/ui/lib/utils";
import { useTranslation } from "react-i18next";

export function MobileThreadPagination({ position, total }: { position: number; total: number }) {
  const { t } = useTranslation();
  if (total < 2 || position < 1 || position > total) return null;
  return (
    <span
      className="ml-2 flex shrink-0 items-center gap-2.5 text-xs text-muted-foreground"
      data-testid="thread-swipe-cue"
    >
      {total <= 7 && (
        <span className="flex items-center gap-1" aria-hidden="true">
          {Array.from({ length: total }, (_, index) => (
            <span
              key={index}
              data-testid="thread-page-dot"
              data-active={index + 1 === position}
              className={cn(
                "h-1 rounded-full",
                index + 1 === position ? "w-3 bg-foreground/70" : "w-1 bg-muted-foreground/30",
              )}
            />
          ))}
        </span>
      )}
      <span className="tabular-nums">{t("threads:threadPosition", { position, total })}</span>
    </span>
  );
}
