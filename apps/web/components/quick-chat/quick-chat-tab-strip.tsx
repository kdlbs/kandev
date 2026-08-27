"use client";

import { useCallback, useMemo, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  DndContext,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  closestCenter,
  type DragEndEvent,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  horizontalListSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { Button } from "@kandev/ui/button";
import { IconX } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { selectQuickChatSessionIsWorking } from "@/lib/state/slices/ui/quick-chat-activity-selectors";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import type { QuickChatSession, QuickTerminalTab } from "@/lib/state/slices/ui/types";
import type { TFunction } from "i18next";
import { cn } from "@/lib/utils";
import { QuickChatTabItem, type QuickChatTabDragProps } from "./quick-chat-tab-item";
import { QuickTerminalTabItem } from "./quick-terminal-tab-item";
import { QuickTabAddMenu } from "./quick-tab-add-menu";
import { conversationTabReference, terminalTabReference } from "./use-quick-chat-tab-order";

const DRAG_ACTIVATION_DISTANCE = 8;
const TOUCH_DRAG_DELAY_MS = 250;
const TOUCH_DRAG_TOLERANCE = 5;

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
  onMoveLeft?: () => void;
  onMoveRight?: () => void;
  canMoveLeft?: boolean;
  canMoveRight?: boolean;
  dragProps?: QuickChatTabDragProps;
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
      dragProps={props.dragProps}
    />
  );
}

type SortableQuickChatTabProps = {
  reference: string;
  children: (dragProps: QuickChatTabDragProps) => ReactNode;
};

function SortableQuickChatTab({ reference, children }: SortableQuickChatTabProps) {
  const {
    attributes,
    listeners,
    setActivatorNodeRef,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: reference });
  return (
    <div
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.55 : undefined,
      }}
      data-testid="quick-chat-sortable-tab"
      data-tab-reference={reference}
      className={cn("shrink-0", isDragging && "z-50")}
    >
      {children({ attributes, listeners, setActivatorNodeRef })}
    </div>
  );
}

function useQuickChatTabDnd(sortableOrder: string[], onTabOrderChange?: (order: string[]) => void) {
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: DRAG_ACTIVATION_DISTANCE } }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: TOUCH_DRAG_DELAY_MS, tolerance: TOUCH_DRAG_TOLERANCE },
    }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const moveTab = useCallback(
    (reference: string, delta: -1 | 1) => {
      if (!onTabOrderChange) return;
      const index = sortableOrder.indexOf(reference);
      const targetIndex = index + delta;
      if (index < 0 || targetIndex < 0 || targetIndex >= sortableOrder.length) return;
      onTabOrderChange(arrayMove(sortableOrder, index, targetIndex));
    },
    [onTabOrderChange, sortableOrder],
  );
  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      if (!onTabOrderChange || !event.over || event.active.id === event.over.id) return;
      const activeIndex = sortableOrder.indexOf(String(event.active.id));
      const overIndex = sortableOrder.indexOf(String(event.over.id));
      if (activeIndex < 0 || overIndex < 0) return;
      onTabOrderChange(arrayMove(sortableOrder, activeIndex, overIndex));
    },
    [onTabOrderChange, sortableOrder],
  );

  return { sensors, moveTab, handleDragEnd };
}

type QuickChatSortableTabItemProps = {
  reference: string;
  session?: QuickChatSession;
  terminal?: QuickTerminalTab;
  name?: string;
  isActive: boolean;
  canMoveLeft: boolean;
  canMoveRight: boolean;
  onTabChange: (sessionId: string) => void;
  onTabClose: (sessionId: string) => void;
  onTerminalClose: (tabId: string) => void;
  onTerminalActivate: (tabId: string) => void;
  onRename: (sessionId: string, name: string) => void;
  moveTab: (reference: string, delta: -1 | 1) => void;
};

function QuickChatSortableTabItem({
  reference,
  session,
  terminal,
  name,
  isActive,
  canMoveLeft,
  canMoveRight,
  onTabChange,
  onTabClose,
  onTerminalClose,
  onTerminalActivate,
  onRename,
  moveTab,
}: QuickChatSortableTabItemProps) {
  return (
    <SortableQuickChatTab reference={reference}>
      {(dragProps) => {
        if (session) {
          return (
            <QuickChatConversationTab
              session={session}
              name={name ?? ""}
              isActive={isActive}
              onActivate={() => onTabChange(session.sessionId)}
              onClose={() => onTabClose(session.sessionId)}
              onRename={(nextName) => onRename(session.sessionId, nextName)}
              onMoveLeft={() => moveTab(reference, -1)}
              onMoveRight={() => moveTab(reference, 1)}
              canMoveLeft={canMoveLeft}
              canMoveRight={canMoveRight}
              dragProps={dragProps}
            />
          );
        }
        if (!terminal) return null;
        return (
          <QuickTerminalTabItem
            sequence={terminal.sequence}
            isActive={isActive}
            error={terminal.error}
            onActivate={() => onTerminalActivate(terminal.tabId)}
            onClose={() => onTerminalClose(terminal.tabId)}
            onMoveLeft={() => moveTab(reference, -1)}
            onMoveRight={() => moveTab(reference, 1)}
            canMoveLeft={canMoveLeft}
            canMoveRight={canMoveRight}
            dragProps={dragProps}
          />
        );
      }}
    </SortableQuickChatTab>
  );
}

type QuickChatTabStripProps = {
  sessions: QuickChatSession[];
  sortableOrder: string[];
  sessionByReference: Map<string, QuickChatSession>;
  terminalByReference: Map<string, QuickTerminalTab>;
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
  moveTab: (reference: string, delta: -1 | 1) => void;
  onCloseModal: () => void;
};

function QuickChatTabStrip({
  sessions,
  sortableOrder,
  sessionByReference,
  terminalByReference,
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
  moveTab,
  onCloseModal,
}: QuickChatTabStripProps) {
  const { t } = useTranslation();
  const setupSessions = sessions.filter((session) => isQuickChatSetupSessionId(session.sessionId));

  return (
    <div className="flex min-w-0 items-center gap-1 border-b bg-muted/20 px-2 py-1">
      <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto scrollbar-hide">
        {sortableOrder.map((reference, index) => {
          const session = sessionByReference.get(reference);
          const terminal = terminalByReference.get(reference);
          if (!session && !terminal) return null;
          const sessionIndex = session
            ? sessions.findIndex((item) => item.sessionId === session.sessionId)
            : -1;
          return (
            <QuickChatSortableTabItem
              key={reference}
              reference={reference}
              session={session}
              terminal={terminal}
              name={session ? quickChatTabName(t, session, sessionIndex) : undefined}
              isActive={
                session
                  ? activeKind === "conversation" && session.sessionId === activeSessionId
                  : activeKind === "terminal" && terminal?.tabId === activeTerminalTabId
              }
              canMoveLeft={index > 0}
              canMoveRight={index < sortableOrder.length - 1}
              onTabChange={onTabChange}
              onTabClose={onTabClose}
              onTerminalClose={onTerminalClose}
              onTerminalActivate={onTerminalActivate}
              onRename={onRename}
              moveTab={moveTab}
            />
          );
        })}
        {setupSessions.map((session, index) => (
          <QuickChatConversationTab
            key={session.sessionId || `new-${index}`}
            session={session}
            name={quickChatTabName(t, session, sessions.indexOf(session))}
            isActive={activeKind === "conversation" && session.sessionId === activeSessionId}
            onActivate={() => onTabChange(session.sessionId)}
            onClose={() => onTabClose(session.sessionId)}
            onRename={(name) => onRename(session.sessionId, name)}
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

type QuickChatTabsProps = {
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
  tabOrderSyncError: string | null;
  tabOrder?: string[];
  onTabOrderChange?: (order: string[]) => void;
};

export function QuickChatTabs({
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
  tabOrderSyncError,
  tabOrder,
  onTabOrderChange,
}: QuickChatTabsProps) {
  const { t } = useTranslation();
  const fallbackOrder = useMemo(
    () => [
      ...sessions
        .filter((session) => !isQuickChatSetupSessionId(session.sessionId))
        .map((session) => conversationTabReference(session.sessionId)),
      ...terminalTabs.map((tab) => terminalTabReference(tab.tabId)),
    ],
    [sessions, terminalTabs],
  );
  const sortableOrder = tabOrder ?? fallbackOrder;
  const sessionByReference = useMemo(
    () =>
      new Map(
        sessions
          .filter((session) => !isQuickChatSetupSessionId(session.sessionId))
          .map((session) => [conversationTabReference(session.sessionId), session]),
      ),
    [sessions],
  );
  const terminalByReference = useMemo(
    () => new Map(terminalTabs.map((tab) => [terminalTabReference(tab.tabId), tab])),
    [terminalTabs],
  );
  const { sensors, moveTab, handleDragEnd } = useQuickChatTabDnd(sortableOrder, onTabOrderChange);
  return (
    <>
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext items={sortableOrder} strategy={horizontalListSortingStrategy}>
          <QuickChatTabStrip
            sessions={sessions}
            sortableOrder={sortableOrder}
            sessionByReference={sessionByReference}
            terminalByReference={terminalByReference}
            activeKind={activeKind}
            activeSessionId={activeSessionId}
            activeTerminalTabId={activeTerminalTabId}
            onTabChange={onTabChange}
            onTabClose={onTabClose}
            onNewChat={onNewChat}
            onNewTerminal={onNewTerminal}
            onTerminalClose={onTerminalClose}
            onTerminalActivate={onTerminalActivate}
            onRename={onRename}
            moveTab={moveTab}
            onCloseModal={onCloseModal}
          />
        </SortableContext>
      </DndContext>
      {tabOrderSyncError && (
        <div
          role="alert"
          className="border-b border-destructive/30 bg-destructive/5 px-3 py-1 text-xs text-destructive"
        >
          <span className="font-medium">{t("chat:quickChatTabOrderSaveFailed")}</span>{" "}
          {t("chat:quickChatTabOrderSaveFailedDescription")}
        </div>
      )}
    </>
  );
}
