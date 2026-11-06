"use client";

import { GridSpinner } from "@/components/grid-spinner";
import { PanelLoadingState } from "@/components/panel-loading-state";
import { useTranslation } from "react-i18next";

export function RunningSpinner() {
  return <GridSpinner className="text-muted-foreground shrink-0 text-[12px]" />;
}

export function PreviewLoadingState({ label }: { label: string }) {
  return <PanelLoadingState testId="preview-loading-state" label={label} />;
}

export function PreviewEmptyState() {
  const { t } = useTranslation();
  return (
    <div className="flex h-full flex-col">
      <div
        className="flex flex-1 items-center justify-center text-sm text-muted-foreground"
        data-testid="preview-empty-state"
      >
        {t("task:noAgentsYet2")}
      </div>
    </div>
  );
}
