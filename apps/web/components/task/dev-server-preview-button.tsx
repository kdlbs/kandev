"use client";

import { useCallback } from "react";
import { IconDeviceDesktop, IconDeviceDesktopOff } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useDevServerPreview } from "@/hooks/domains/session/use-dev-server-preview";

type DevServerPreviewButtonProps = {
  /** Dockview group the dev-server output panel should open in. */
  outputGroupId?: string | null;
  className: string;
  iconClassName: string;
  /** Distinguishes the terminal-group control from the chat-group one in specs. */
  testId?: string;
};

/**
 * Start/stop toggle for the repository dev script.
 *
 * Starting also opens the browser preview and the Dev Server output panel, so
 * the script's stdout/stderr is visible somewhere. Stopping leaves both panels
 * in place: the buffered output is usually the reason the user wants to stop.
 */
export function DevServerPreviewButton({
  outputGroupId,
  className,
  iconClassName,
  testId = "dev-server-preview-toggle",
}: DevServerPreviewButtonProps) {
  const { t } = useTranslation();
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const focusOrAddBrowserPanel = useDockviewStore((s) => s.focusOrAddBrowserPanel);
  const addDevServerPanel = useDockviewStore((s) => s.addDevServerPanel);
  const dev = useDevServerPreview(activeSessionId);

  const handleClick = useCallback(async () => {
    if (!activeSessionId) return;
    if (dev.isRunning) {
      await dev.stop();
      return;
    }
    focusOrAddBrowserPanel();
    addDevServerPanel(outputGroupId ?? undefined);
    await dev.start();
  }, [activeSessionId, dev, focusOrAddBrowserPanel, addDevServerPanel, outputGroupId]);

  const label = dev.isRunning ? t("task:stopDevServerPreview") : t("task:startDevServerPreview");

  return (
    <Button
      size="sm"
      variant="ghost"
      className={className}
      onClick={handleClick}
      disabled={dev.isPending || !activeSessionId}
      aria-label={label}
      title={label}
      data-testid={testId}
    >
      {dev.isRunning ? (
        <IconDeviceDesktopOff className={iconClassName} />
      ) : (
        <IconDeviceDesktop className={iconClassName} />
      )}
    </Button>
  );
}
