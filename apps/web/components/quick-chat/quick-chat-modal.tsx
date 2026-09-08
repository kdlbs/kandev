"use client";

import { memo, useMemo, useRef, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import dynamic from "@/lib/routing/client-dynamic";
import { useAppStore } from "@/components/state-provider";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import { QuickChatDeleteDialog } from "./quick-chat-delete-dialog";
import { QuickChatSessionView } from "./quick-chat-session-view";
import { QuickChatSetup } from "./quick-chat-setup";
import { QuickChatTabs } from "./quick-chat-tab-strip";
import { useQuickChatModal } from "./use-quick-chat-modal";
import { useQuickChatWidth } from "@/hooks/use-quick-chat-width";
import {
  ClarificationEscapeGuardProvider,
  type ClarificationEscapePredicate,
  type ClarificationEscapeGuardRegistry,
} from "@/hooks/use-clarification-escape-guard";
import { ConfigChatSetup } from "@/components/config-chat/config-chat-setup";
import { useConfigChat } from "@/components/config-chat/use-config-chat";

const QuickTerminalTabView = dynamic(
  () => import("./quick-terminal-tab-view").then((module) => module.QuickTerminalTabView),
  { ssr: false },
);

type QuickChatModalProps = {
  workspaceId: string;
};

type QuickChatContentProps = {
  workspaceId: string;
  configChat: ReturnType<typeof useConfigChat>;
  quickChat: ReturnType<typeof useQuickChatModal>;
  setQuickChatInitialPrompt: (sessionId: string, prompt?: string) => void;
};

function QuickChatContent({
  workspaceId,
  configChat,
  quickChat,
  setQuickChatInitialPrompt,
}: QuickChatContentProps) {
  const canCreateConfigurationChat = !quickChat.sessions.some(
    (session) => session.kind === "config",
  );
  const setupKind =
    quickChat.activeSession && isQuickChatSetupSessionId(quickChat.activeSession.sessionId)
      ? quickChat.activeSession.kind
      : null;

  return (
    <>
      <QuickChatTabs
        sessions={quickChat.sessions}
        terminalTabs={quickChat.terminalTabs}
        activeKind={quickChat.activeKind}
        activeSessionId={quickChat.activeSessionId}
        activeTerminalTabId={quickChat.activeTerminalTabId}
        onTabChange={quickChat.setActiveQuickChatSession}
        onTabClose={quickChat.handleCloseTab}
        onNewChat={quickChat.handleNewChat}
        onNewTerminal={quickChat.handleNewTerminal}
        onTerminalActivate={quickChat.handleActivateTerminal}
        onTerminalClose={quickChat.handleCloseTerminal}
        onRename={quickChat.handleRename}
        onCloseModal={() => quickChat.handleOpenChange(false)}
        tabOrderSyncError={quickChat.tabOrderSyncError}
        tabOrder={quickChat.tabOrder}
        onTabOrderChange={quickChat.persistTabOrder}
      />
      {quickChat.activeKind === "terminal" && quickChat.activeTerminalTab && (
        <QuickTerminalTabView
          key={quickChat.activeTerminalTab.tabId}
          tab={quickChat.activeTerminalTab}
          onStateChange={(state) =>
            quickChat.handleTerminalStateChange(quickChat.activeTerminalTab!.tabId, state)
          }
          onDescriptorReady={(descriptor) =>
            quickChat.handleTerminalDescriptorReady(quickChat.activeTerminalTab!.tabId, descriptor)
          }
        />
      )}
      {quickChat.activeKind === "conversation" &&
        quickChat.activeSessionId &&
        quickChat.activeSession &&
        !quickChat.activeSessionNeedsAgent && (
          <QuickChatSessionView
            session={quickChat.activeSession}
            onInitialPromptSent={() =>
              setQuickChatInitialPrompt(quickChat.activeSessionId!, undefined)
            }
          />
        )}
      {quickChat.activeKind === "conversation" &&
        quickChat.activeSessionNeedsAgent &&
        setupKind === "chat" && (
          <QuickChatSetup
            key={`${workspaceId}:${quickChat.setupKey}`}
            workspaceId={workspaceId}
            canCreateConfigurationChat={canCreateConfigurationChat}
            pendingAgentId={quickChat.pendingAgentId}
            onStart={quickChat.handleSelectAgent}
            onCancel={() => quickChat.handleOpenChange(false)}
            onKindChange={quickChat.handleSetupKindChange}
          />
        )}
      {quickChat.activeKind === "conversation" &&
        quickChat.activeSessionNeedsAgent &&
        setupKind === "config" && (
          <ConfigChatSetup
            key={`${workspaceId}:config:${quickChat.setupKey}`}
            defaultProfileId={configChat.defaultProfileId}
            isStarting={configChat.isStarting}
            error={configChat.error}
            onStart={(profileId, prompt) => configChat.startSession(profileId, prompt)}
            onCancel={() => quickChat.handleOpenChange(false)}
            onKindChange={quickChat.handleSetupKindChange}
          />
        )}
    </>
  );
}

function QuickChatResizeHandle({
  edge,
  onMouseDown,
}: {
  edge: "left" | "right";
  onMouseDown: (event: React.MouseEvent) => void;
}) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      tabIndex={-1}
      aria-label={
        edge === "left" ? t("chat:resizeQuickChatFromLeft") : t("chat:resizeQuickChatFromRight")
      }
      data-testid={`quick-chat-resize-${edge}`}
      onMouseDown={onMouseDown}
      className={`group absolute inset-y-0 z-20 hidden w-2 cursor-ew-resize items-center justify-center sm:flex ${
        edge === "left" ? "left-0" : "right-0"
      }`}
    >
      <span
        className={`absolute inset-y-0 w-px bg-transparent transition-colors group-hover:bg-primary/60 ${
          edge === "left" ? "-left-px" : "-right-px"
        }`}
      />
    </button>
  );
}

export const QuickChatModal = memo(function QuickChatModal({ workspaceId }: QuickChatModalProps) {
  const { t } = useTranslation();
  const configChat = useConfigChat(workspaceId);
  const quickChat = useQuickChatModal(workspaceId, configChat.reset);
  const setQuickChatInitialPrompt = useAppStore((state) => state.setQuickChatInitialPrompt);
  const { width, leftResizeHandleProps, rightResizeHandleProps } = useQuickChatWidth();
  // A ref, not state: several widgets (a pending clarification, an open
  // suggestion popup, the reverse-search overlay) can be registered at once
  // and none of their register/unregister calls should trigger a re-render of
  // the modal itself -- onEscapeKeyDown reads the live map only when Escape
  // actually fires.
  const escapeGuardsRef = useRef(new Map<string, ClarificationEscapePredicate>());
  const escapeGuardRegistry = useMemo<ClarificationEscapeGuardRegistry>(
    () => ({
      register: (id, predicate) => escapeGuardsRef.current.set(id, predicate),
      unregister: (id) => escapeGuardsRef.current.delete(id),
    }),
    [],
  );
  return (
    <ClarificationEscapeGuardProvider value={escapeGuardRegistry}>
      <Dialog open={quickChat.isOpen} onOpenChange={quickChat.handleOpenChange}>
        <DialogContent
          className="!left-0 !top-0 !h-dvh !max-h-dvh !w-screen !max-w-none !translate-x-0 !translate-y-0 flex flex-col gap-0 p-0 pt-safe pb-safe shadow-2xl sm:!left-1/2 sm:!top-1/2 sm:!h-[85vh] sm:!max-h-[85vh] sm:!w-[var(--quick-chat-width)] sm:!max-w-[calc(100vw-2rem)] sm:!-translate-x-1/2 sm:!-translate-y-1/2"
          style={{ "--quick-chat-width": `${width}px` } as CSSProperties}
          showCloseButton={false}
          overlayClassName="bg-black/20 sm:bg-black/40 sm:backdrop-blur-sm"
          onEscapeKeyDown={(event) => {
            // A pending, expanded clarification (or an open suggestion popup,
            // or the reverse-search overlay) handles this Escape itself and
            // the modal stays open -- but only when that widget's own guard
            // predicate reports it will actually act on this exact keydown
            // (matching its enabled/scope/modifier state), so Escape never
            // goes silently swallowed with nothing left to handle it. Once no
            // guard claims it (e.g. a second, now-unguarded Escape), the modal
            // closes, matching the main task chat panel's two-stage Escape
            // after #2729.
            for (const test of escapeGuardsRef.current.values()) {
              if (test(event)) {
                event.preventDefault();
                break;
              }
            }
          }}
        >
          <DialogTitle className="sr-only">{t("common:commandQuickChat")}</DialogTitle>
          <QuickChatResizeHandle edge="left" {...leftResizeHandleProps} />
          <QuickChatResizeHandle edge="right" {...rightResizeHandleProps} />
          <QuickChatContent
            workspaceId={workspaceId}
            configChat={configChat}
            quickChat={quickChat}
            setQuickChatInitialPrompt={setQuickChatInitialPrompt}
          />
        </DialogContent>
      </Dialog>

      <QuickChatDeleteDialog
        sessionToDelete={quickChat.sessionToClose}
        onOpenChange={(open) => !open && quickChat.setSessionToClose(null)}
        onConfirm={quickChat.handleConfirmClose}
      />
    </ClarificationEscapeGuardProvider>
  );
});
