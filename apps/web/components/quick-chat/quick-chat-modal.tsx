"use client";

import { memo, useCallback, useMemo, useRef, type CSSProperties } from "react";
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
import { useConfigChat } from "@/components/config-chat/use-config-chat";
import {
  MobileConfirmationHost,
  MobileConfirmationHostBody,
} from "@/components/confirmation/mobile-confirmation-host";
import type { QuickChatInitialPrompt } from "@/lib/state/slices/ui/types";
import { useQuickChatSetupDraft } from "./use-quick-chat-setup-draft";
import { useQuickChatCreation, type QuickChatCreationActions } from "./use-quick-chat-creation";

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
  setQuickChatInitialPrompt: (sessionId: string, prompt?: QuickChatInitialPrompt) => void;
  setupDraft: ReturnType<typeof useQuickChatSetupDraft>;
  creationActions: QuickChatCreationActions;
  discardSetupDraft: () => void;
  onCloseModal: () => void;
  onRegisterSetupDiscard: (discard: () => void) => () => void;
};

type QuickChatActiveContentProps = Omit<
  QuickChatContentProps,
  "discardSetupDraft" | "onCloseModal"
>;

type QuickChatModalShellProps = QuickChatContentProps & {
  isOpen: boolean;
  width: number;
  leftResizeHandleProps: ReturnType<typeof useQuickChatWidth>["leftResizeHandleProps"];
  rightResizeHandleProps: ReturnType<typeof useQuickChatWidth>["rightResizeHandleProps"];
  onOpenChange: (open: boolean) => void;
  sessionToDelete: ReturnType<typeof useQuickChatModal>["sessionToClose"];
  onDeleteDialogOpenChange: (open: boolean) => void;
  onConfirmDelete: () => void;
};

function QuickChatContent({
  workspaceId,
  configChat,
  quickChat,
  setQuickChatInitialPrompt,
  setupDraft,
  creationActions,
  discardSetupDraft,
  onCloseModal,
  onRegisterSetupDiscard,
}: QuickChatContentProps) {
  return (
    <>
      <QuickChatTabs
        sessions={quickChat.sessions}
        terminalTabs={quickChat.terminalTabs}
        activeKind={quickChat.activeKind}
        activeSessionId={quickChat.activeSessionId}
        activeTerminalTabId={quickChat.activeTerminalTabId}
        onTabChange={quickChat.setActiveQuickChatSession}
        onTabClose={(sessionId) => {
          if (
            quickChat.activeKind === "conversation" &&
            sessionId === quickChat.activeSessionId &&
            isQuickChatSetupSessionId(sessionId)
          ) {
            discardSetupDraft();
          }
          quickChat.handleCloseTab(sessionId);
        }}
        onNewChat={() => {
          if (
            quickChat.activeKind === "conversation" &&
            quickChat.activeSessionId &&
            isQuickChatSetupSessionId(quickChat.activeSessionId)
          ) {
            discardSetupDraft();
          }
          quickChat.handleNewChat();
        }}
        onNewTerminal={quickChat.handleNewTerminal}
        onTerminalActivate={quickChat.handleActivateTerminal}
        onTerminalClose={quickChat.handleCloseTerminal}
        onRename={quickChat.handleRename}
        onCloseModal={onCloseModal}
        tabOrderSyncError={quickChat.tabOrderSyncError}
        tabOrder={quickChat.tabOrder}
        onTabOrderChange={quickChat.persistTabOrder}
      />
      <QuickChatActiveContent
        workspaceId={workspaceId}
        configChat={configChat}
        quickChat={quickChat}
        setQuickChatInitialPrompt={setQuickChatInitialPrompt}
        setupDraft={setupDraft}
        creationActions={creationActions}
        onRegisterSetupDiscard={onRegisterSetupDiscard}
      />
    </>
  );
}

function QuickChatActiveContent({
  workspaceId,
  configChat,
  quickChat,
  setQuickChatInitialPrompt,
  setupDraft,
  creationActions,
  onRegisterSetupDiscard,
}: QuickChatActiveContentProps) {
  const canCreateConfigurationChat = !quickChat.sessions.some(
    (session) => session.kind === "config" && !isQuickChatSetupSessionId(session.sessionId),
  );

  if (quickChat.pendingQuickChatOpen) return <QuickChatSelectionLoading />;

  return (
    <>
      <QuickChatTerminalContent quickChat={quickChat} />
      <QuickChatConversationContent
        workspaceId={workspaceId}
        configChat={configChat}
        quickChat={quickChat}
        setQuickChatInitialPrompt={setQuickChatInitialPrompt}
        setupDraft={setupDraft}
        creationActions={creationActions}
        onRegisterSetupDiscard={onRegisterSetupDiscard}
        canCreateConfigurationChat={canCreateConfigurationChat}
      />
    </>
  );
}

function QuickChatSelectionLoading() {
  const { t } = useTranslation();
  return (
    <div
      aria-live="polite"
      className="flex min-h-0 flex-1 items-center justify-center text-sm text-muted-foreground"
      data-testid="quick-chat-selection-loading"
    >
      {t("common:loading")}
    </div>
  );
}

function QuickChatTerminalContent({
  quickChat,
}: {
  quickChat: QuickChatContentProps["quickChat"];
}) {
  if (quickChat.activeKind !== "terminal" || !quickChat.activeTerminalTab) return null;
  const tab = quickChat.activeTerminalTab;
  return (
    <QuickTerminalTabView
      key={tab.tabId}
      tab={tab}
      onStateChange={(state) => quickChat.handleTerminalStateChange(tab.tabId, state)}
      onDescriptorReady={(descriptor) =>
        quickChat.handleTerminalDescriptorReady(tab.tabId, descriptor)
      }
    />
  );
}

function QuickChatConversationContent({
  workspaceId,
  configChat,
  quickChat,
  setQuickChatInitialPrompt,
  setupDraft,
  creationActions,
  onRegisterSetupDiscard,
  canCreateConfigurationChat,
}: QuickChatActiveContentProps & { canCreateConfigurationChat: boolean }) {
  if (
    quickChat.activeKind !== "conversation" ||
    !quickChat.activeSessionId ||
    !quickChat.activeSession
  ) {
    return null;
  }
  const setupKind = isQuickChatSetupSessionId(quickChat.activeSession.sessionId)
    ? quickChat.activeSession.kind
    : null;
  if (!quickChat.activeSessionNeedsAgent) {
    return (
      <QuickChatSessionView
        session={quickChat.activeSession}
        onInitialPromptAttempted={() => {
          const sessionId = quickChat.activeSessionId!;
          setQuickChatInitialPrompt(sessionId, undefined);
          const pending = setupDraft.pendingCreation;
          if (pending?.sessionId === sessionId)
            setupDraft.updatePendingCreation(pending.creationId, { messageAttempted: true });
        }}
      />
    );
  }
  if (setupKind) {
    return (
      <QuickChatSetup
        key={`${workspaceId}:${quickChat.setupKey}`}
        workspaceId={workspaceId}
        kind={setupKind}
        canCreateConfigurationChat={canCreateConfigurationChat}
        defaultConfigProfileId={configChat.defaultProfileId}
        pendingAgentId={quickChat.pendingAgentId}
        configurationStarting={configChat.isStarting}
        configurationError={configChat.error}
        quickChatError={quickChat.setupError}
        pendingCreation={creationActions.pending}
        checkingCreation={creationActions.checking}
        onCheckCreation={() => creationActions.check()}
        onRetryCreation={() => creationActions.retry()}
        onDeleteCreation={() => creationActions.remove()}
        draft={setupDraft.draft}
        onDraftChange={setupDraft.update}
        onStartQuickChat={creationActions.start}
        onStartConfigChat={async (agentId, payload) => {
          const sessionId = await configChat.startSession(agentId, payload, {
            setupSessionId: quickChat.activeSessionId ?? undefined,
          });
          if (sessionId) setupDraft.clear(false);
          return Boolean(sessionId);
        }}
        onKindChange={quickChat.handleSetupKindChange}
        onDiscardDraft={() => {
          if (!setupDraft.pendingCreation) setupDraft.clear(false);
        }}
        onRegisterDiscard={onRegisterSetupDiscard}
      />
    );
  }
  return null;
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

function QuickChatModalShell({
  workspaceId,
  configChat,
  quickChat,
  setQuickChatInitialPrompt,
  setupDraft,
  creationActions,
  discardSetupDraft,
  onCloseModal,
  onRegisterSetupDiscard,
  isOpen,
  width,
  leftResizeHandleProps,
  rightResizeHandleProps,
  onOpenChange,
  sessionToDelete,
  onDeleteDialogOpenChange,
  onConfirmDelete,
}: QuickChatModalShellProps) {
  const { t } = useTranslation();
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
      <MobileConfirmationHost open={isOpen} key={workspaceId}>
        {({ contentProps }) => (
          <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent
              {...contentProps}
              className="!left-0 !top-0 !h-dvh !max-h-dvh !w-screen !max-w-none !translate-x-0 !translate-y-0 flex min-h-0 flex-col gap-0 p-0 pt-safe pb-safe shadow-2xl sm:!left-1/2 sm:!top-1/2 sm:!h-[85dvh] sm:!max-h-[85dvh] sm:!w-[var(--quick-chat-width)] sm:!max-w-[calc(100vw-2rem)] sm:!-translate-x-1/2 sm:!-translate-y-1/2"
              style={{ "--quick-chat-width": `${width}px` } as CSSProperties}
              showCloseButton={false}
              overlayClassName="bg-black/20 sm:bg-black/40 sm:backdrop-blur-sm"
              onEscapeKeyDown={(event) =>
                handleQuickChatEscape(event, contentProps, escapeGuardsRef.current)
              }
            >
              <MobileConfirmationHostBody>
                <DialogTitle className="sr-only">{t("common:commandQuickChat")}</DialogTitle>
                <QuickChatResizeHandle edge="left" {...leftResizeHandleProps} />
                <QuickChatResizeHandle edge="right" {...rightResizeHandleProps} />
                <QuickChatContent
                  workspaceId={workspaceId}
                  configChat={configChat}
                  quickChat={quickChat}
                  setQuickChatInitialPrompt={setQuickChatInitialPrompt}
                  setupDraft={setupDraft}
                  creationActions={creationActions}
                  discardSetupDraft={discardSetupDraft}
                  onCloseModal={onCloseModal}
                  onRegisterSetupDiscard={onRegisterSetupDiscard}
                />
              </MobileConfirmationHostBody>
            </DialogContent>
          </Dialog>
        )}
      </MobileConfirmationHost>
      <QuickChatDeleteDialog
        sessionToDelete={sessionToDelete}
        onOpenChange={onDeleteDialogOpenChange}
        onConfirm={onConfirmDelete}
      />
    </ClarificationEscapeGuardProvider>
  );
}

function handleQuickChatEscape(
  event: KeyboardEvent,
  contentProps: { onEscapeKeyDown?: (event: KeyboardEvent) => void },
  guards: Map<string, ClarificationEscapePredicate>,
) {
  if (contentProps.onEscapeKeyDown) {
    contentProps.onEscapeKeyDown(event);
    return;
  }
  for (const test of guards.values()) {
    if (test(event)) {
      event.preventDefault();
      break;
    }
  }
}

export const QuickChatModal = memo(function QuickChatModal({ workspaceId }: QuickChatModalProps) {
  const configChat = useConfigChat(workspaceId);
  const setupDraftUserId = useAppStore((state) => state.auth?.user?.id ?? "local");
  const setupDraft = useQuickChatSetupDraft(workspaceId, setupDraftUserId);
  const setQuickChatInitialPrompt = useAppStore((state) => state.setQuickChatInitialPrompt);
  const setupDiscardRef = useRef<(() => void) | null>(null);
  const registerSetupDiscard = useCallback((discard: () => void) => {
    setupDiscardRef.current = discard;
    return () => {
      if (setupDiscardRef.current === discard) setupDiscardRef.current = null;
    };
  }, []);
  const discardSetupDraft = useCallback(() => {
    if (setupDraft.pendingCreation) return;
    if (setupDiscardRef.current) setupDiscardRef.current();
    else setupDraft.clear(true);
  }, [setupDraft.clear, setupDraft.pendingCreation]);
  const quickChat = useQuickChatModal(workspaceId, configChat.reset, discardSetupDraft);
  const creationActions = useQuickChatCreation({
    workspaceId,
    userId: setupDraftUserId,
    quickChat,
    setupDraft,
  });
  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) discardSetupDraft();
      quickChat.handleOpenChange(open);
    },
    [discardSetupDraft, quickChat.handleOpenChange],
  );
  const width = useQuickChatWidth();
  const onCloseModal = useCallback(() => handleOpenChange(false), [handleOpenChange]);
  const onDeleteDialogOpenChange = useCallback(
    (open: boolean) => !open && quickChat.setSessionToClose(null),
    [quickChat.setSessionToClose],
  );
  return (
    <QuickChatModalShell
      workspaceId={workspaceId}
      configChat={configChat}
      quickChat={quickChat}
      setQuickChatInitialPrompt={setQuickChatInitialPrompt}
      setupDraft={setupDraft}
      creationActions={creationActions}
      discardSetupDraft={discardSetupDraft}
      onCloseModal={onCloseModal}
      onRegisterSetupDiscard={registerSetupDiscard}
      isOpen={quickChat.isOpen}
      width={width.width}
      leftResizeHandleProps={width.leftResizeHandleProps}
      rightResizeHandleProps={width.rightResizeHandleProps}
      onOpenChange={handleOpenChange}
      sessionToDelete={quickChat.sessionToClose}
      onDeleteDialogOpenChange={onDeleteDialogOpenChange}
      onConfirmDelete={quickChat.handleConfirmClose}
    />
  );
});
