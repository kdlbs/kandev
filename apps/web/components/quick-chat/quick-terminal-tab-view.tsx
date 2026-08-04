"use client";

import { useCallback } from "react";
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

/** Selected terminal content for the shared Quick Chat surface. */
export function QuickTerminalTabView({ tab, onStateChange }: Props) {
  const startSession = useCallback<StartPtySession>(
    (size, options) => startHostShell(size, options),
    [],
  );

  return (
    <div
      className="min-h-0 flex-1 overflow-hidden"
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
        className="h-full min-h-0 rounded-md bg-[#0b0b0c] p-2 overflow-hidden"
        onStateChange={onStateChange}
      />
    </div>
  );
}
