"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { startHostShell } from "@/lib/api";
import {
  PtyTerminalView,
  type PtyTerminalState,
  type StartPtySession,
} from "@/components/settings/pty-terminal-view";
import type { QuickTerminalTab } from "@/lib/state/slices/ui/types";

export type QuickTerminalTabDescriptor = QuickTerminalTab;

type Props = {
  tab: QuickTerminalTab;
  onStateChange: (state: PtyTerminalState) => void;
};

function QuickTerminalLifecycleStatus({ tab }: { tab: QuickTerminalTab }) {
  const { t } = useTranslation();
  if (tab.status === "connecting") {
    return (
      <p
        role="status"
        aria-live="polite"
        data-testid="quick-terminal-status"
        className="shrink-0 px-2 pb-1 text-xs text-muted-foreground"
      >
        {t("sidebar:quickChatTerminalConnecting")}
      </p>
    );
  }

  if (tab.status === "error" || tab.error) {
    return (
      <p
        role="alert"
        data-testid="quick-terminal-status"
        className="shrink-0 px-2 pb-1 text-xs text-destructive"
      >
        {t("sidebar:quickChatTerminalError", {
          error: tab.error || t("sidebar:quickChatTerminalUnknownError"),
        })}
      </p>
    );
  }

  if (tab.status === "exited") {
    return (
      <p
        role="status"
        aria-live="polite"
        data-testid="quick-terminal-status"
        className="shrink-0 px-2 pb-1 text-xs text-muted-foreground"
      >
        {tab.exitCode == null
          ? t("sidebar:quickChatTerminalExited")
          : t("sidebar:quickChatTerminalExitedWithCode", { code: tab.exitCode })}
      </p>
    );
  }

  return null;
}

/** Selected terminal content for the shared Quick Chat surface. */
export function QuickTerminalTabView({ tab, onStateChange }: Props) {
  const startSession = useCallback<StartPtySession>(
    (size, options) => startHostShell(size, options),
    [],
  );

  return (
    <div
      className="flex min-h-0 flex-1 flex-col overflow-hidden"
      data-testid="quick-terminal-tab-panel"
      data-terminal-tab-id={tab.tabId}
    >
      <PtyTerminalView
        startSession={startSession}
        sessionId={tab.sessionId}
        clientId={tab.tabId}
        ownerId={tab.tabId}
        lifecycle="detach-on-unmount"
        testIdPrefix="quick-terminal"
        className="h-full min-h-0 flex-1 rounded-md bg-[#0b0b0c] p-2 overflow-hidden"
        onStateChange={onStateChange}
      />
      <QuickTerminalLifecycleStatus tab={tab} />
    </div>
  );
}
