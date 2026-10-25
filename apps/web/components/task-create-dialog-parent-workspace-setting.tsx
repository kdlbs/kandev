"use client";

import { useEffect, useRef, useState } from "react";
import { IconInfoCircle } from "@tabler/icons-react";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { InitialWorkspaceLayout } from "@/lib/types/http";
import type { InitialWorkspaceLayoutMode } from "@/components/task-create-dialog-types";

export type TaskCreateParentWorkspaceSettingProps = {
  initialWorkspaceLayout: InitialWorkspaceLayout;
  onInitialWorkspaceLayoutChange: (next: InitialWorkspaceLayout) => void;
  initialWorkspaceLayoutMode: InitialWorkspaceLayoutMode;
};

function ParentWorkspaceInfo({
  label,
  title,
  description,
}: {
  label: string;
  title: string;
  description: string;
}) {
  const usesTouchDrawer = useTouchDrawer();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const trigger = (
    <button
      type="button"
      className={`${controlSizingClassName("icon")} cursor-pointer rounded-md border-0 bg-transparent p-0 text-muted-foreground/70 hover:bg-transparent hover:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring`}
      aria-label={label}
      aria-expanded={usesTouchDrawer ? drawerOpen : undefined}
      aria-haspopup={usesTouchDrawer ? "dialog" : undefined}
      data-testid="task-create-initial-workspace-layout-info"
    >
      <IconInfoCircle className="h-3.5 w-3.5" aria-hidden="true" />
    </button>
  );

  if (usesTouchDrawer) {
    return (
      <Drawer open={drawerOpen} onOpenChange={setDrawerOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent
          data-testid="task-create-initial-workspace-layout-help-drawer"
          className="max-h-[80dvh]"
        >
          <DrawerHeader>
            <DrawerTitle>{title}</DrawerTitle>
            <DrawerDescription>{description}</DrawerDescription>
          </DrawerHeader>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>{trigger}</TooltipTrigger>
      <TooltipContent side="top" className="z-[60] max-w-xs">
        {description}
      </TooltipContent>
    </Tooltip>
  );
}

export function TaskCreateParentWorkspaceSetting({
  initialWorkspaceLayout,
  onInitialWorkspaceLayoutChange,
  initialWorkspaceLayoutMode,
}: TaskCreateParentWorkspaceSettingProps) {
  const { t } = useTranslation();
  const layoutForcedByMultipleRepositories = useRef(false);

  useEffect(() => {
    if (initialWorkspaceLayoutMode === "multiple-repositories") {
      layoutForcedByMultipleRepositories.current = true;
      if (initialWorkspaceLayout !== "task_root") {
        onInitialWorkspaceLayoutChange("task_root");
      }
      return;
    }
    if (initialWorkspaceLayoutMode === "single-repository") {
      if (layoutForcedByMultipleRepositories.current) {
        layoutForcedByMultipleRepositories.current = false;
        if (initialWorkspaceLayout !== "repository") {
          onInitialWorkspaceLayoutChange("repository");
        }
      }
      return;
    }
    layoutForcedByMultipleRepositories.current = false;
    if (initialWorkspaceLayout !== "repository") {
      onInitialWorkspaceLayoutChange("repository");
    }
  }, [initialWorkspaceLayout, initialWorkspaceLayoutMode, onInitialWorkspaceLayoutChange]);

  if (initialWorkspaceLayoutMode === "unavailable") return null;

  const infoLabel = t("task:initialWorkspaceLayoutInfoLabel");
  const infoTitle = t("task:initialWorkspaceLayoutTitle");
  const info = t("task:initialWorkspaceLayoutInfo");
  return (
    <div
      className="flex min-h-11 min-w-0 items-center gap-1 text-xs text-muted-foreground md:min-h-6"
      data-testid="task-create-initial-workspace-layout-setting"
    >
      {initialWorkspaceLayoutMode === "multiple-repositories" ? (
        <span data-testid="task-create-initial-workspace-layout-multiple">
          {t("task:initialWorkspaceLayoutMultiple")}
        </span>
      ) : (
        <>
          <Checkbox
            id="task-create-initial-workspace-layout"
            checked={initialWorkspaceLayout === "task_root"}
            onCheckedChange={(checked) =>
              onInitialWorkspaceLayoutChange(checked === true ? "task_root" : "repository")
            }
            className="[@media(pointer:coarse)]:size-5"
            data-testid="task-create-initial-workspace-layout-checkbox"
          />
          <label htmlFor="task-create-initial-workspace-layout" className="cursor-pointer">
            {t("task:initialWorkspaceLayoutLabel")}
          </label>
        </>
      )}
      <ParentWorkspaceInfo label={infoLabel} title={infoTitle} description={info} />
    </div>
  );
}
