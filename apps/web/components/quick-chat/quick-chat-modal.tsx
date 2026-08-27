"use client";

import { memo, useMemo, useRef, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { IconX } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import dynamic from "@/lib/routing/client-dynamic";
import { QuickChatDeleteDialog } from "./quick-chat-delete-dialog";
import { QuickChatSessionView } from "./quick-chat-session-view";
import { QuickChatTabItem } from "./quick-chat-tab-item";
import { QuickTerminalTabItem } from "./quick-terminal-tab-item";
import { QuickTabAddMenu } from "./quick-tab-add-menu";
import { QuickChatSetup } from "./quick-chat-setup";
import { useQuickChatModal } from "./use-quick-chat-modal";
import { useQuickChatWidth } from "@/hooks/use-quick-chat-width";
import {
  ClarificationEscapeGuardProvider,
  type ClarificationEscapePredicate,
  type ClarificationEscapeGuardRegistry,
} from "@/hooks/use-clarification-escape-guard";
import { ConfigChatSetup } from "@/components/config-chat/config-chat-setup";
import { useConfigChat } from "@/components/config-chat/use-config-chat";
import type { QuickChatSession, QuickTerminalTab } from "@/lib/state/slices/ui/types";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import { selectQuickChatSessionIsWorking } from "@/lib/state/slices/ui/quick-chat-activity-selectors";
import type { TFunction } from "i18next";

const QuickTerminalTabView = dynamic(
  () => import("./quick-terminal-tab-view").then((module) => module.QuickTerminalTabView),
  { ssr: false },
);

type QuickChatModalProps = {
  workspaceId: string;
};

function quickChatTabName(t: TFunction, session: QuickChatSession, index: number) {
  if (!isQuickChatSetupSessionId(session.sessionId)) {
    return session.name || t("chat:chatTabName", { index: index + 1 });
  }
  return session.kind === "config" ? t("chat:configurationChatTab") : t("chat:newChatTab");
}

function QuickChatConversationTab({
  session,
  ...props
}: {
  session: QuickChatSession;
  name: string;
  isActive: boolean;
  onActivate: () => void;
  onClose: () => void;
  onRename: (name: string) => void;
}) {
  const isWorking = useAppStore((state) =>
    selectQuickChatSessionIsWorking(state, session.sessionId),
  );

  return (
    <QuickChatTabItem
      {...props}
      isRenameable={!isQuickChatSetupSessionId(session.sessionId)}
      isWorking={!isQuickChatSetupSessionId(session.sessionId) && isWorking}
      kind={session.kind}
    />
  );
}

function QuickChatTabs({
  sessions,
  terminalTabs,
  activeKind,
  activeSessionId,
  activeTerminalTabId,
  onTabChange,
  onTabClose,
  onNewChat,
  onNewTerminal,
  onTerminalClose,
  onTerminalActivate,
  onRename,
  onCloseModal,
}: {
  sessions: QuickChatSession[];
  terminalTabs: QuickTerminalTab[];
  activeKind: "conversation" | "terminal";
  activeSessionId: string | null;
  activeTerminalTabId: string | null;
  onTabChange: (sessionId: string) => void;
  onTabClose: (sessionId: string) => void;
  onNewChat: () => void;
  onNewTerminal: () => void;
  onTerminalClose: (tabId: string) => void;
  onTerminalActivate: (tabId: string) => void;
  onRename: (sessionId: string, name: string) => void;
  onCloseModal: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-1 px-2 py-1 border-b bg-muted/20">
      <div className="flex items-center gap-1 overflow-x-auto flex-1 scrollbar-hide">
        {sessions.map((s, index) => {
          const tabName = quickChatTabName(t, s, index);
          return (
            <QuickChatConversationTab
              key={s.sessionId || `new-${index}`}
              session={s}
              name={tabName}
              isActive={activeKind === "conversation" && s.sessionId === activeSessionId}
              onActivate={() => onTabChange(s.sessionId)}
              onClose={() => onTabClose(s.sessionId)}
              onRename={(name) => onRename(s.sessionId, name)}
            />
          );
        })}
        {terminalTabs.map((tab) => (
          <QuickTerminalTabItem
            key={tab.tabId}
            sequence={tab.sequence}
            isActive={activeKind === "terminal" && tab.tabId === activeTerminalTabId}
            error={tab.error}
            onActivate={() => onTerminalActivate(tab.tabId)}
            onClose={() => onTerminalClose(tab.tabId)}
          />
        ))}
        <QuickTabAddMenu onNewAgent={onNewChat} onNewTerminal={onNewTerminal} />
      </div>
      {/* Touch devices have no Escape key or visible overlay to dismiss the
          full-screen dialog, so give them an explicit close control. */}
      <Button
        size="sm"
        variant="ghost"
        className="h-11 w-11 shrink-0 cursor-pointer p-0 sm:hidden"
        onClick={onCloseModal}
        aria-label={t("sidebar:quickChatClose")}
        data-testid="quick-chat-close"
      >
        <IconX className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

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
          overlayClassName="bg-black/20"
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
