"use client";

import { useTranslation } from "react-i18next";
import { IconLayoutList, IconLayoutRows } from "@tabler/icons-react";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import type { WorkflowListView } from "@/hooks/domains/settings/use-workflow-list-view";

const ITEM_CLASS =
  "cursor-pointer gap-1.5 px-2.5 data-[state=on]:bg-muted data-[state=on]:text-foreground max-md:min-h-11 [@media(pointer:coarse)]:min-h-11";

export function WorkflowListViewToggle({
  view,
  onViewChange,
}: {
  view: WorkflowListView;
  onViewChange: (view: WorkflowListView) => void;
}) {
  const { t } = useTranslation();
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      value={view}
      onValueChange={(value) => {
        if (value === "compact" || value === "detailed") onViewChange(value);
      }}
      aria-label={t("workflows:listViewLabel")}
      data-testid="workflow-list-view-toggle"
    >
      <ToggleGroupItem
        value="compact"
        className={ITEM_CLASS}
        data-testid="workflow-list-view-compact"
      >
        <IconLayoutList className="h-4 w-4" aria-hidden="true" />
        {t("workflows:listViewCompact")}
      </ToggleGroupItem>
      <ToggleGroupItem
        value="detailed"
        className={ITEM_CLASS}
        data-testid="workflow-list-view-detailed"
      >
        <IconLayoutRows className="h-4 w-4" aria-hidden="true" />
        {t("workflows:listViewDetailed")}
      </ToggleGroupItem>
    </ToggleGroup>
  );
}
