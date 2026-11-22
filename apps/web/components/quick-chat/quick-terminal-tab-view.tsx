"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { createQuickTerminalTab, startHostShell, toQuickTerminalTab } from "@/lib/api";
import { Button } from "@kandev/ui/button";
import {
  PtyTerminalView,
  type PtyTerminalState,
  type StartPtySession,
} from "@/components/settings/pty-terminal-view";
import type { QuickTerminalTab } from "@/lib/state/slices/ui/types";
import type { QuickTerminalRecoveryPhase } from "@/hooks/domains/session/use-quick-terminal-lifecycle";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useVisualViewportOffset } from "@/hooks/use-visual-viewport-offset";

export type QuickTerminalTabDescriptor = QuickTerminalTab;

type Props = {
  tab: QuickTerminalTab;
  onStateChange: (state: PtyTerminalState) => void;
  onDescriptorReady?: (tab: QuickTerminalTab) => void;
  onRecover?: () => void;
  recoveryGeneration?: number;
  recoveryPhase?: QuickTerminalRecoveryPhase;
  recoveryClosing?: boolean;
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

function useQuickTerminalDescriptor(
  tab: QuickTerminalTab,
  shouldCreateDescriptor: boolean,
  onStateChange: Props["onStateChange"],
  onDescriptorReady: Props["onDescriptorReady"],
) {
  const [descriptorReady, setDescriptorReady] = useState(!shouldCreateDescriptor);
  const onStateChangeRef = useRef(onStateChange);
  const onDescriptorReadyRef = useRef(onDescriptorReady);
  onStateChangeRef.current = onStateChange;
  onDescriptorReadyRef.current = onDescriptorReady;

  useEffect(() => {
    if (!shouldCreateDescriptor) {
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
        onDescriptorReadyRef.current?.(
          nextTab.sessionId
            ? nextTab
            : { ...nextTab, status: "connecting", exitCode: undefined, error: undefined },
        );
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
  }, [shouldCreateDescriptor, tab.tabId, tab.workspaceId]);

  return descriptorReady;
}

function QuickTerminalRecoveryAction({
  tab,
  recoveryPhase,
  recoveryClosing,
  onRecover,
}: {
  tab: QuickTerminalTab;
  recoveryPhase?: QuickTerminalRecoveryPhase;
  recoveryClosing?: boolean;
  onRecover?: () => void;
}) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const isRecoverable =
    !recoveryClosing &&
    (tab.status === "exited" || (!tab.sessionId && (tab.status === "error" || Boolean(tab.error))));
  if (!isRecoverable && !recoveryPhase) return null;

  const recoveryPending = recoveryClosing || recoveryPhase !== undefined;
  let className = "h-12 min-h-12 shrink-0 px-4";
  if (isMobile) className = "h-12 min-h-12 w-full shrink-0";
  else if (isFinePointer) className = "h-7 min-h-7 shrink-0 px-2 text-xs";

  return (
    <div className="shrink-0 px-2 pb-2">
      <Button
        type="button"
        variant="outline"
        data-testid="quick-terminal-recovery"
        className={className}
        disabled={recoveryPending}
        aria-busy={recoveryPending}
        onClick={onRecover}
      >
        {t("sidebar:quickChatTerminalStartNewShell")}
      </Button>
    </div>
  );
}

/** Selected terminal content for the shared Quick Chat surface. */
export function QuickTerminalTabView({
  tab,
  onStateChange,
  onDescriptorReady,
  onRecover,
  recoveryGeneration = 0,
  recoveryPhase,
  recoveryClosing,
}: Props) {
  const { isMobile } = useResponsiveBreakpoint();
  const { bottomOffset } = useVisualViewportOffset();
  const onStateChangeRef = useRef(onStateChange);
  onStateChangeRef.current = onStateChange;
  const needsDescriptor = !tab.sessionId && tab.status === "connecting";
  const shouldCreateDescriptor = needsDescriptor && recoveryGeneration === 0 && !recoveryClosing;
  const descriptorReady = useQuickTerminalDescriptor(
    tab,
    shouldCreateDescriptor,
    onStateChange,
    onDescriptorReady,
  );

  const shouldRenderTerminal =
    !recoveryClosing &&
    (Boolean(tab.sessionId) ||
      (needsDescriptor && descriptorReady && recoveryPhase !== "preparing"));
  const startSession = useCallback<StartPtySession>(async (size, options) => {
    const session = await startHostShell(size, options);
    onStateChangeRef.current({
      status: session.running ? "running" : "exited",
      sessionId: session.session_id,
      exitCode: session.exit_code ?? null,
      error: null,
    });
    return session;
  }, []);

  return (
    <div
      className="flex min-h-0 flex-1 flex-col overflow-hidden"
      data-testid="quick-terminal-tab-panel"
      data-terminal-tab-id={tab.tabId}
      style={isMobile ? { paddingBottom: bottomOffset } : undefined}
    >
      {shouldRenderTerminal && (
        <PtyTerminalView
          key={recoveryGeneration}
          startSession={startSession}
          sessionId={tab.sessionId}
          clientId={tab.tabId}
          ownerId={tab.tabId}
          lifecycle="detach-on-unmount"
          testIdPrefix="quick-terminal"
          className="h-full min-h-0 flex-1 rounded-md bg-[#0b0b0c] p-2 overflow-hidden"
          onStateChange={onStateChange}
          mobileControls={isMobile && tab.status === "running"}
        />
      )}
      <QuickTerminalLifecycleStatus tab={tab} />
      <QuickTerminalRecoveryAction
        tab={tab}
        recoveryPhase={recoveryPhase}
        recoveryClosing={recoveryClosing}
        onRecover={onRecover}
      />
    </div>
  );
}
