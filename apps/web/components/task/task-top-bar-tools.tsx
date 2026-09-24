"use client";

import { IconBug, IconTool } from "@tabler/icons-react";
import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { isDebugUI } from "@/lib/config";
import { TaskChromeDisclosure } from "./task-chrome-disclosure";
import { LayoutPresetSelector } from "./layout-preset-selector";
import { EditorsMenu } from "./editors-menu";
import { OpenTaskFolderButton } from "./open-task-folder-button";

export function TaskTopBarTools({
  activeSessionId,
  isArchived,
  embeddedVscodeSupported,
  showDebugOverlay,
  onToggleDebugOverlay,
}: {
  activeSessionId?: string | null;
  isArchived?: boolean;
  embeddedVscodeSupported?: boolean;
  showDebugOverlay?: boolean;
  onToggleDebugOverlay?: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const showDebugToggle = isDebugUI() && onToggleDebugOverlay;
  if (isArchived && !showDebugToggle) return null;

  return (
    <TaskChromeDisclosure
      label={t("task:taskTools")}
      icon={<IconTool className="size-4" aria-hidden />}
      testId="task-tools-trigger"
      showLabel
      open={open}
      onOpenChange={setOpen}
    >
      <div className="space-y-3">
        {!isArchived && (
          <>
            <div className="flex items-center justify-between gap-3">
              <span className="text-xs text-muted-foreground">{t("settings:layout")}</span>
              <LayoutPresetSelector onLayoutSelected={() => setOpen(false)} />
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-xs text-muted-foreground">{t("common:workspace")}</span>
              <div className="flex items-center gap-1">
                <EditorsMenu
                  activeSessionId={activeSessionId ?? null}
                  embeddedVscodeSupported={embeddedVscodeSupported ?? false}
                  onEditorOpened={() => setOpen(false)}
                />
                <OpenTaskFolderButton sessionId={activeSessionId ?? null} />
              </div>
            </div>
          </>
        )}
        {showDebugToggle && (
          <Button
            variant="ghost"
            size="sm"
            className="w-full cursor-pointer justify-start gap-2 text-xs"
            onClick={onToggleDebugOverlay}
            aria-pressed={showDebugOverlay}
          >
            <IconBug className="size-4" aria-hidden />
            {showDebugOverlay ? t("task:hideDebugInfo") : t("task:showDebugInfo")}
          </Button>
        )}
      </div>
    </TaskChromeDisclosure>
  );
}
