"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { IconChevronRight, IconMessage2, IconPlus, IconUsers } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useConversationFork } from "@/hooks/domains/task/use-conversation-fork";
import type { ConversationForkSelection } from "@/hooks/domains/task/use-conversation-fork";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { generateUUID } from "@/lib/uuid";
import type { Message } from "@/lib/types/http";
import { useAppStore } from "@/components/state-provider";
import type { TaskCreateDialogProps } from "@/components/task-create-dialog-types";
import { ConversationForkTaskDestinations } from "./conversation-fork-task-destinations";
import { NewSessionDialog } from "./new-session-dialog";
import type { ConversationForkFormContext } from "./conversation-fork-types";

type ConversationForkFlowProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  message: Message;
  groupId?: string;
};

type Destination = "task" | "child_task" | "agent";

const DEFAULT_SELECTION: ConversationForkSelection = {
  includeToolEvidence: false,
  attachmentIds: [],
};

function DestinationRow({
  destination,
  selected,
  disabled,
  onSelect,
}: {
  destination: Destination;
  selected: boolean;
  disabled: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  const title = {
    task: t("task:conversationForkNewTask"),
    child_task: t("task:conversationForkChildTask"),
    agent: t("task:conversationForkNewAgent"),
  }[destination];
  const description = {
    task: t("task:conversationForkSeparateWorkspace"),
    child_task: t("task:conversationForkChildWorkspaceChoice"),
    agent: t("task:conversationForkSharedWorkspace"),
  }[destination];
  const Icon = destination === "agent" ? IconUsers : IconPlus;
  return (
    <button
      type="button"
      data-testid={`conversation-fork-destination-${destination}`}
      aria-pressed={selected}
      disabled={disabled}
      onClick={onSelect}
      className={`flex min-h-11 w-full cursor-pointer items-center gap-3 rounded-md border px-3 py-2 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${selected ? "border-primary bg-primary/5" : "border-border hover:bg-muted/50"}`}
    >
      <Icon className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium">{title}</span>
        <span className="block text-xs text-muted-foreground">{description}</span>
      </span>
      <IconChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
    </button>
  );
}

function ForkSourceSummary({ message }: { message: Message }) {
  const { t } = useTranslation();
  const role =
    message.author_type === "user"
      ? t("task:conversationForkUser")
      : t("task:conversationForkAssistant");
  const at = new Date(message.created_at).toLocaleTimeString();
  return (
    <div className="rounded-md bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
      {t("task:conversationForkThrough", { role, at })}
    </div>
  );
}

function ForkPickerBody({
  source,
  cutoffMessage,
  sourceLoading,
  sourceError,
  selectedDestination,
  onSelectDestination,
  onRetry,
  canFork,
}: {
  source: ReturnType<typeof useConversationFork>["source"];
  cutoffMessage: Message;
  sourceLoading: boolean;
  sourceError: ReturnType<typeof useConversationFork>["sourceError"];
  selectedDestination: Destination | null;
  onSelectDestination: (destination: Destination) => void;
  onRetry: () => void;
  canFork: boolean;
}) {
  const { t } = useTranslation();
  if (sourceLoading) {
    return (
      <div role="status" className="rounded-md bg-muted/40 px-3 py-4 text-sm text-muted-foreground">
        {t("task:conversationForkPreparing")}
      </div>
    );
  }
  if (sourceError) {
    return (
      <div className="grid gap-3" role="alert">
        <p className="text-sm text-destructive">{t("task:conversationForkLoadFailed")}</p>
        <Button type="button" variant="outline" onClick={onRetry} className="cursor-pointer">
          {t("task:conversationForkRetry")}
        </Button>
      </div>
    );
  }
  if (!source) return null;
  return (
    <div className="grid gap-3">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <IconMessage2 className="h-4 w-4" aria-hidden="true" />
        <span className="min-w-0 truncate">{source.title}</span>
      </div>
      <ForkSourceSummary message={cutoffMessage} />
      {!canFork && (
        <p className="text-xs text-destructive">{t("task:conversationForkUnfinished")}</p>
      )}
      <div className="grid gap-2" role="group" aria-label={t("task:conversationForkDestination")}>
        <DestinationRow
          destination="task"
          selected={selectedDestination === "task"}
          disabled={!canFork}
          onSelect={() => onSelectDestination("task")}
        />
        <DestinationRow
          destination="child_task"
          selected={selectedDestination === "child_task"}
          disabled={!canFork}
          onSelect={() => onSelectDestination("child_task")}
        />
        <DestinationRow
          destination="agent"
          selected={selectedDestination === "agent"}
          disabled={!canFork}
          onSelect={() => onSelectDestination("agent")}
        />
      </div>
    </div>
  );
}

// eslint-disable-next-line max-lines-per-function
export function ConversationForkFlow({
  open,
  onOpenChange,
  message,
  groupId,
}: ConversationForkFlowProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const fork = useConversationFork();
  const sourceTask = useAppStore((state) =>
    state.kanban.tasks.find((task) => task.id === message.task_id),
  );
  const workspaceId = useAppStore((state) => sourceTask?.workspaceId ?? state.workspaces.activeId);
  const steps = useAppStore((state): TaskCreateDialogProps["steps"] => {
    if (!sourceTask) return EMPTY_TASK_CREATE_STEPS;
    return (
      state.kanbanMulti.snapshots[sourceTask.workflowId]?.steps ??
      (state.kanban.workflowId === sourceTask.workflowId
        ? state.kanban.steps
        : EMPTY_TASK_CREATE_STEPS)
    );
  });
  const [pickerOpen, setPickerOpen] = useState(open);
  const [newSessionOpen, setNewSessionOpen] = useState(false);
  const [taskDestination, setTaskDestination] = useState<"task" | "child_task" | null>(null);
  const [destination, setDestination] = useState<Destination | null>(null);
  const [selection, setSelection] = useState(DEFAULT_SELECTION);
  const [creationRequestId, setCreationRequestId] = useState("");
  const [forkRemoved, setForkRemoved] = useState(false);
  const consumedRef = useRef(false);
  const messageId = message.id;
  const sessionId = message.session_id;
  const canFork = Boolean(
    fork.source && (message.author_type === "user" || fork.source.cutoffTurnComplete),
  );

  useEffect(() => {
    if (!open) {
      setPickerOpen(false);
      setNewSessionOpen(false);
      setTaskDestination(null);
      return;
    }
    consumedRef.current = false;
    setForkRemoved(false);
    setDestination(null);
    setSelection(DEFAULT_SELECTION);
    setPickerOpen(true);
    void fork.reset();
    void fork.loadSource(sessionId, messageId);
  }, [fork.loadSource, fork.reset, messageId, open, sessionId]);

  const closeFlow = useCallback(async () => {
    if (!consumedRef.current) await fork.discardSnapshot();
    fork.reset();
    setPickerOpen(false);
    setNewSessionOpen(false);
    setTaskDestination(null);
    onOpenChange(false);
  }, [fork.discardSnapshot, fork.reset, onOpenChange]);

  const handleContinue = useCallback(async () => {
    if (!destination || !canFork) return;
    const snapshot = await fork.createSnapshot(DEFAULT_SELECTION);
    if (!snapshot) return;
    setSelection(DEFAULT_SELECTION);
    setCreationRequestId(generateUUID());
    if (destination === "agent") setNewSessionOpen(true);
    else setTaskDestination(destination);
    setPickerOpen(false);
  }, [canFork, destination, fork.createSnapshot]);

  const handleApplySelection = useCallback(
    async (nextSelection: ConversationForkSelection, forceNew = false) => {
      const snapshot = await fork.createSnapshot(nextSelection, { forceNew });
      if (!snapshot) return false;
      if (forceNew || JSON.stringify(nextSelection) !== JSON.stringify(selection)) {
        setCreationRequestId(generateUUID());
      }
      setSelection(nextSelection);
      return true;
    },
    [fork.createSnapshot, selection],
  );

  const onRangeStartChange = useCallback(
    (startMessageId?: string) => void fork.loadAttachments(startMessageId),
    [fork.loadAttachments],
  );
  const onModelChange = useCallback(
    (modelId: string) => void fork.refreshEstimate(modelId),
    [fork.refreshEstimate],
  );
  const onConsumed = useCallback(() => {
    consumedRef.current = true;
  }, []);
  const removeFork = useCallback(async () => {
    await fork.discardSnapshot();
    setForkRemoved(true);
  }, [fork.discardSnapshot]);
  const conversationFork: ConversationForkFormContext | undefined = useMemo(
    () =>
      !forkRemoved && fork.source && fork.snapshot
        ? {
            source: fork.source,
            snapshot: fork.snapshot,
            snapshotError: fork.snapshotError,
            selection,
            creationRequestId,
            attachmentsLoading: fork.attachmentsLoading,
            onPreview: () => undefined,
            onRemove: () => void removeFork(),
            onApplySelection: handleApplySelection,
            onRangeStartChange,
            onModelChange,
            onConsumed,
          }
        : undefined,
    [
      closeFlow,
      creationRequestId,
      forkRemoved,
      fork.attachmentsLoading,
      fork.snapshot,
      fork.source,
      handleApplySelection,
      onConsumed,
      removeFork,
      onModelChange,
      onRangeStartChange,
      selection,
    ],
  );

  const retrySource = () => void fork.loadSource(sessionId, messageId);
  const pickerContent = (
    <div className="grid gap-4">
      <ForkPickerBody
        source={fork.source}
        cutoffMessage={message}
        sourceLoading={fork.sourceLoading}
        sourceError={fork.sourceError}
        selectedDestination={destination}
        onSelectDestination={setDestination}
        onRetry={retrySource}
        canFork={canFork}
      />
      {fork.snapshotError && (
        <div
          role="alert"
          className="rounded-md border border-destructive/50 bg-destructive/5 p-3 text-sm"
        >
          {t("task:conversationForkCreateFailed")}
        </div>
      )}
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          variant="ghost"
          onClick={() => void closeFlow()}
          className="cursor-pointer"
        >
          {t("common:cancel")}
        </Button>
        <Button
          type="button"
          onClick={() => void handleContinue()}
          disabled={
            fork.snapshotLoading ||
            fork.sourceLoading ||
            !canFork ||
            !destination ||
            (destination !== "agent" && !sourceTask)
          }
          className="cursor-pointer"
        >
          {fork.snapshotLoading ? t("task:conversationForkPreparing") : t("common:continue")}
        </Button>
      </div>
    </div>
  );

  return (
    <>
      {isMobile ? (
        <Drawer
          open={pickerOpen}
          onOpenChange={(next) => {
            setPickerOpen(next);
            if (!next && !newSessionOpen) void closeFlow();
          }}
        >
          <DrawerContent
            className="max-h-[80dvh] pb-[env(safe-area-inset-bottom)]"
            data-testid="conversation-fork-picker"
          >
            <DrawerHeader className="text-left">
              <DrawerTitle>{t("task:conversationForkTitle")}</DrawerTitle>
              <DrawerDescription>{t("task:conversationForkChooseDestination")}</DrawerDescription>
            </DrawerHeader>
            <div className="min-h-0 overflow-y-auto px-4 pb-4">{pickerContent}</div>
          </DrawerContent>
        </Drawer>
      ) : (
        <Dialog
          open={pickerOpen}
          onOpenChange={(next) => {
            setPickerOpen(next);
            if (!next && !newSessionOpen) void closeFlow();
          }}
        >
          <DialogContent className="min-w-0 sm:max-w-lg" data-testid="conversation-fork-picker">
            <DialogHeader>
              <DialogTitle>{t("task:conversationForkTitle")}</DialogTitle>
              <DialogDescription>{t("task:conversationForkChooseDestination")}</DialogDescription>
            </DialogHeader>
            {pickerContent}
          </DialogContent>
        </Dialog>
      )}
      <NewSessionDialog
        open={newSessionOpen}
        onOpenChange={(next) => {
          setNewSessionOpen(next);
          if (!next) void closeFlow();
        }}
        taskId={message.task_id}
        groupId={groupId}
        conversationFork={conversationFork}
      />
      {sourceTask && taskDestination && (
        <ConversationForkTaskDestinations
          destination={taskDestination}
          sourceTask={sourceTask}
          workspaceId={workspaceId}
          steps={steps}
          conversationFork={conversationFork}
          onClose={() => void closeFlow()}
        />
      )}
    </>
  );
}

const EMPTY_TASK_CREATE_STEPS: TaskCreateDialogProps["steps"] = [];
