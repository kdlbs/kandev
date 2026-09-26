"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { createQuickTerminalTab, startHostShell, toQuickTerminalTab } from "@/lib/api";
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
  onDescriptorReady?: (tab: QuickTerminalTab) => void;
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
export function QuickTerminalTabView({ tab, onStateChange, onDescriptorReady }: Props) {
  const needsDescriptor = !tab.sessionId && tab.status === "connecting";
  const [descriptorReady, setDescriptorReady] = useState(!needsDescriptor);
  const onStateChangeRef = useRef(onStateChange);
  const onDescriptorReadyRef = useRef(onDescriptorReady);
  onStateChangeRef.current = onStateChange;
  onDescriptorReadyRef.current = onDescriptorReady;

  useEffect(() => {
    if (!needsDescriptor) {
      setDescriptorReady(true);
      return undefined;
    }

    let cancelled = false;
    setDescriptorReady(false);
    createQuickTerminalTab(tab.tabId, tab.workspaceId)
      .then((descriptor) => {
        // The descriptor is keyed by the stable tabId and the backend create is
        // idempotent, so a cancelled mount (StrictMode remount, tab switch,
        // reload) must NOT delete it — the next mount reuses the same row. Only
        // an explicit tab close deletes the descriptor, via useQuickTerminalClose.
        if (cancelled) return;
        const nextTab = toQuickTerminalTab(descriptor);
        onDescriptorReadyRef.current?.(nextTab);
        setDescriptorReady(true);
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        onStateChangeRef.current({
          status: "error",
          sessionId: null,
          exitCode: null,
          error: error instanceof Error ? error.message : String(error),
        });
      });

    return () => {
      cancelled = true;
    };
  }, [needsDescriptor, tab.tabId, tab.workspaceId]);

  const shouldRenderTerminal = Boolean(tab.sessionId) || (needsDescriptor && descriptorReady);
  const startSession = useCallback<StartPtySession>(
    async (size, options) => {
      const session = await startHostShell(size, options);
      onStateChange({
        status: session.running ? "running" : "exited",
        sessionId: session.session_id,
        exitCode: session.exit_code ?? null,
        error: null,
      });
      return session;
    },
    [onStateChange],
  );

  return (
    <div
      className="flex min-h-0 flex-1 flex-col overflow-hidden"
      data-testid="quick-terminal-tab-panel"
      data-terminal-tab-id={tab.tabId}
    >
      {shouldRenderTerminal && (
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
      )}
      <QuickTerminalLifecycleStatus tab={tab} />
    </div>
  );
}
