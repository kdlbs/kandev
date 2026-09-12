"use client";

import { IconAdjustments, IconCheck, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { threadViewName } from "@/lib/state/slices/ui/thread-view-builtins";
import type { ThreadView } from "@/lib/state/slices/ui/thread-view-types";

export function MobileThreadViewList({
  activeView,
  views,
  hasDraft,
  admittedCount,
  matchingCount,
  hiddenCount,
  disabledReason,
  onSelect,
  onNewView,
  onOpenSettings,
}: {
  activeView: ThreadView;
  views: ThreadView[];
  hasDraft: boolean;
  admittedCount: number;
  matchingCount: number;
  hiddenCount: number;
  disabledReason: string | null;
  onSelect: (id: string) => void;
  onNewView: () => void;
  onOpenSettings: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3 p-3" data-testid="threads-mobile-view-list">
      <div className="rounded-lg bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
        {t("threads:columnsSummary", { admitted: admittedCount, matching: matchingCount })}
        {hiddenCount > 0 && (
          <span className="ml-1">{t("threads:hiddenCount", { count: hiddenCount })}</span>
        )}
      </div>
      {hasDraft && <ThreadViewDraftHint />}
      <div className="space-y-1">
        {views.map((view) => (
          <Button
            key={view.id}
            type="button"
            variant="ghost"
            disabled={hasDraft}
            className="min-h-11 w-full cursor-pointer justify-start gap-2 px-3 text-left text-sm"
            onClick={() => onSelect(view.id)}
            data-testid={`threads-mobile-view-option-${view.id}`}
          >
            <IconCheck className={view.id === activeView.id ? "h-4 w-4" : "h-4 w-4 opacity-0"} />
            <span className="truncate">{threadViewName(view, t)}</span>
          </Button>
        ))}
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        <Button
          type="button"
          variant="outline"
          className="min-h-11 cursor-pointer justify-start"
          onClick={onNewView}
          disabled={!!disabledReason}
          title={disabledReason ?? undefined}
          data-testid="threads-mobile-new-view"
        >
          <IconPlus className="mr-2 h-4 w-4" />
          {t("threads:newView")}
        </Button>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 cursor-pointer justify-start"
          onClick={onOpenSettings}
          data-testid="threads-mobile-view-settings"
        >
          <IconAdjustments className="mr-2 h-4 w-4" />
          {t("threads:viewSettings")}
        </Button>
      </div>
    </div>
  );
}

export function ThreadViewDraftHint() {
  const { t } = useTranslation();
  return (
    <p role="status" className="px-2 py-1.5 text-xs text-muted-foreground">
      {t("threads:saveOrDiscardBeforeSwitchingView")}
    </p>
  );
}
