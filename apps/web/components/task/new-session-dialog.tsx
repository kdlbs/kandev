"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { FormEvent, KeyboardEvent, RefObject } from "react";
import { DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";

import { AgentSelector, TaskFormInputs } from "@/components/task-create-dialog-selectors";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";
import { useSummarizeSession } from "@/hooks/use-summarize-session";
import { useTaskExecutorProfile } from "@/hooks/domains/session/use-task-executor-profile";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { ExecutorProfile } from "@/lib/types/http";
import { buildHandoffInitialState, type HandoffPreset } from "./handoff-types";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { PromptResultRecovery } from "@/components/prompt-result-recovery";
import { EnvironmentBadges, ContextSelect } from "./session-dialog-shared";
import {
  activateNewSession,
  handoffProfileLabel,
  shouldDisableSubmit,
  useConversationForkModelEstimate,
  useNewSessionDialogState,
  useSessionContextChange,
  useSessionLaunchSubmit,
  useSessionOptions,
  useSessionProfileSelection,
  useSessionPromptController,
} from "./new-session-form-actions";
import { Trans, useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { ConversationForkChip } from "./conversation-fork-chip";
import { ConversationForkPreview } from "./conversation-fork-preview";
import type { ConversationForkFormContext } from "./conversation-fork-types";
import { NewSessionDialogSurface } from "./new-session-dialog-surface";

export type { HandoffPreset } from "./handoff-types";
export { useSessionPromptController };

const PROGRAMMATIC_SUBMIT_EVENT = { preventDefault: () => {} } as unknown as FormEvent;

type NewSessionDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: string;
  workspaceId?: string | null;
  groupId?: string;
  handoff?: HandoffPreset;
  conversationFork?: ConversationForkFormContext;
};

function SessionFormHeader({
  executorLabel,
  worktreeBranch,
  noCompatibleProfiles,
  hasProfiles,
  executorProfileName,
  showAgentSelector,
  profileOptions,
  selectedProfileId,
  isCreating,
  onProfileChange,
}: {
  executorLabel: string | null;
  worktreeBranch: string | null;
  noCompatibleProfiles: boolean;
  hasProfiles: boolean;
  executorProfileName: string | null;
  showAgentSelector: boolean;
  profileOptions: ReturnType<typeof useAgentProfileOptions>;
  selectedProfileId: string;
  isCreating: boolean;
  onProfileChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <EnvironmentBadges executorLabel={executorLabel} worktreeBranch={worktreeBranch} />
      <NoAgentBanner
        noCompatibleProfiles={noCompatibleProfiles}
        hasProfiles={hasProfiles}
        executorProfileName={executorProfileName}
      />
      {showAgentSelector && (
        <div className="min-w-0 space-y-1.5">
          <label className="text-xs font-medium text-muted-foreground">
            {t("task:agentProfile")}
          </label>
          <AgentSelector
            options={profileOptions}
            value={selectedProfileId}
            onValueChange={onProfileChange}
            disabled={isCreating}
            placeholder={t("task:selectAgentProfile")}
            popoverPortal
          />
        </div>
      )}
    </>
  );
}

type SessionProfileSelection = ReturnType<typeof useSessionProfileSelection>;
type SessionOption = ReturnType<typeof useSessionOptions>[number];
type SessionPromptController = ReturnType<typeof useSessionPromptController>;

function handleSessionFormKeyDown(
  event: KeyboardEvent,
  canSubmit: boolean,
  handleSubmit: (event: FormEvent) => void,
) {
  const isSubmitShortcut = event.key === "Enter" && (event.metaKey || event.ctrlKey);
  if (!isSubmitShortcut || !canSubmit) return;
  event.preventDefault();
  void handleSubmit(event as unknown as FormEvent);
}

type SessionFormFieldsProps = {
  isMobile: boolean;
  taskId: string;
  workspaceId?: string | null;
  executorLabel: string | null;
  executorProfile: ExecutorProfile | null;
  worktreeBranch: string | null;
  initialPrompt: string | null;
  profileSelection: SessionProfileSelection;
  isCreating: boolean;
  conversationFork?: ConversationForkFormContext;
  contextValue: string;
  handleContextChange: (value: string) => void;
  sessionOptions: SessionOption[];
  isSummarizing: boolean;
  hasPrompt: boolean;
  hasPendingAttachmentUploads: boolean;
  promptRef: RefObject<TaskFormInputsHandle | null>;
  setHasPrompt: (hasPrompt: boolean) => void;
  setHasPendingAttachmentUploads: (pending: boolean) => void;
  isBusyState: boolean;
  handleSubmit: (event: FormEvent) => void;
  handleEnhancePrompt: () => void;
  isEnhancingPrompt: boolean;
  isUtilityConfigured: boolean;
  pendingResult: SessionPromptController["pendingResult"];
  applyPending: SessionPromptController["applyPending"];
  copyPending: SessionPromptController["copyPending"];
};

function SessionFormFields({
  isMobile,
  taskId,
  workspaceId,
  executorLabel,
  executorProfile,
  worktreeBranch,
  initialPrompt,
  profileSelection,
  isCreating,
  conversationFork,
  contextValue,
  handleContextChange,
  sessionOptions,
  isSummarizing,
  hasPrompt,
  hasPendingAttachmentUploads,
  promptRef,
  setHasPrompt,
  setHasPendingAttachmentUploads,
  isBusyState,
  handleSubmit,
  handleEnhancePrompt,
  isEnhancingPrompt,
  isUtilityConfigured,
  pendingResult,
  applyPending,
  copyPending,
}: SessionFormFieldsProps) {
  return (
    <div
      className={
        isMobile
          ? "min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4"
          : "min-w-0 space-y-4"
      }
    >
      <SessionFormHeader
        executorLabel={executorLabel}
        worktreeBranch={worktreeBranch}
        noCompatibleProfiles={profileSelection.noCompatibleProfiles}
        hasProfiles={profileSelection.hasProfiles}
        executorProfileName={executorProfile?.name ?? null}
        showAgentSelector={profileSelection.showAgentSelector}
        profileOptions={profileSelection.profileOptions}
        selectedProfileId={profileSelection.selectedProfileId}
        isCreating={isCreating}
        onProfileChange={profileSelection.onProfileChange}
      />
      {conversationFork && <ConversationForkChip fork={conversationFork} />}
      {!conversationFork && (
        <ContextSelect
          value={contextValue}
          onValueChange={handleContextChange}
          hasInitialPrompt={!!initialPrompt}
          sessionOptions={sessionOptions}
          isSummarizing={isSummarizing}
        />
      )}
      <TaskFormInputs
        isSessionMode
        taskId={taskId}
        workspaceId={workspaceId}
        autoFocus={!isMobile}
        initialDescription=""
        onDescriptionChange={setHasPrompt}
        onPendingAttachmentUploadsChange={setHasPendingAttachmentUploads}
        onKeyDown={(event) =>
          handleSessionFormKeyDown(
            event,
            !isBusyState &&
              !hasPendingAttachmentUploads &&
              hasPrompt &&
              profileSelection.hasProfiles,
            handleSubmit,
          )
        }
        descriptionValueRef={promptRef}
        disabled={isBusyState}
        onEnhancePrompt={handleEnhancePrompt}
        isEnhancingPrompt={isEnhancingPrompt}
        isUtilityConfigured={isUtilityConfigured}
        onComposerSubmit={() => {
          if (isBusyState || hasPendingAttachmentUploads || !profileSelection.hasProfiles) {
            return false;
          }
          void handleSubmit(PROGRAMMATIC_SUBMIT_EVENT);
          return true;
        }}
      />
      <PromptResultRecovery
        pendingResult={pendingResult}
        onApply={applyPending}
        onCopy={copyPending}
      />
    </div>
  );
}

function SessionFormFooter({
  isMobile,
  isCreating,
  isSubmitDisabled,
  onClose,
}: {
  isMobile: boolean;
  isCreating: boolean;
  isSubmitDisabled: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <DialogFooter
      className={
        isMobile
          ? "shrink-0 border-t px-4 py-3 pb-[calc(0.75rem+env(safe-area-inset-bottom,0px))]"
          : undefined
      }
    >
      <Button
        type="button"
        variant="ghost"
        onClick={onClose}
        disabled={isCreating}
        className="cursor-pointer"
      >
        {t("common:cancel")}
      </Button>
      <Button type="submit" disabled={isSubmitDisabled} className="cursor-pointer">
        {isCreating ? t("task:creatingEllipsis") : t("task:startAgent2")}
      </Button>
    </DialogFooter>
  );
}

type NewSessionFormProps = {
  taskId: string;
  workspaceId?: string | null;
  currentProfileId: string;
  executorId: string;
  executorLabel: string | null;
  executorProfile: ExecutorProfile | null;
  worktreeBranch: string | null;
  initialPrompt: string | null;
  agentProfiles: AgentProfileOption[];
  groupId?: string;
  handoff?: HandoffPreset;
  conversationFork?: ConversationForkFormContext;
  onClose: () => void;
};

function useSessionFormController({
  taskId,
  currentProfileId,
  executorId,
  executorProfile,
  initialPrompt,
  agentProfiles,
  groupId,
  handoff,
  conversationFork,
  onClose,
}: NewSessionFormProps) {
  const handoffInitial = handoff ? buildHandoffInitialState(handoff) : null;
  const { toast } = useToast();
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const { summarize, isSummarizing } = useSummarizeSession();
  const [contextValue, setContextValue] = useState(handoffInitial?.contextValue ?? "blank");
  const [hasPrompt, setHasPrompt] = useState(false);
  const [hasPendingAttachmentUploads, setHasPendingAttachmentUploads] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const promptRef = useRef<TaskFormInputsHandle | null>(null);
  const busySignal = Number(isCreating) + Number(isSummarizing);
  const isBusyState = busySignal > 0;
  const sessionOptions = useSessionOptions(taskId);
  const isUtilityConfigured = useIsUtilityConfigured();
  const profileSelection = useSessionProfileSelection({
    agentProfiles,
    executorProfile,
    currentProfileId,
    handoff,
  });
  const selectedModelId =
    agentProfiles.find((profile) => profile.id === profileSelection.selectedProfileId)?.model ?? "";
  useConversationForkModelEstimate(conversationFork, selectedModelId);
  const { handleEnhancePrompt, isEnhancingPrompt, pendingResult, applyPending, copyPending } =
    useSessionPromptController(promptRef, taskId);
  const handleContextChange = useSessionContextChange({
    promptRef,
    initialPrompt,
    summarize,
    toast,
    setContextValue,
    setHasPrompt,
  });

  const handleSubmit = useSessionLaunchSubmit({
    promptRef,
    taskId,
    selectedProfileId: profileSelection.selectedProfileId,
    profileExplicit: profileSelection.profileExplicit,
    executorId,
    contextValue,
    initialPrompt,
    agentProfiles,
    groupId,
    onClose,
    toast,
    setActiveSession,
    activateSession: activateNewSession,
    setIsCreating,
    conversationForkId: conversationFork?.snapshot.descriptor.id,
    creationRequestId: conversationFork?.creationRequestId,
    onForkConsumed: conversationFork?.onConsumed,
  });
  const isSubmitDisabled =
    shouldDisableSubmit(isBusyState, hasPrompt, profileSelection.hasProfiles) ||
    hasPendingAttachmentUploads;

  return {
    conversationFork,
    contextValue,
    handleContextChange,
    sessionOptions,
    isSummarizing,
    hasPrompt,
    hasPendingAttachmentUploads,
    promptRef,
    setHasPrompt,
    setHasPendingAttachmentUploads,
    isBusyState,
    handleSubmit,
    handleEnhancePrompt,
    isEnhancingPrompt,
    isUtilityConfigured,
    pendingResult,
    applyPending,
    copyPending,
    profileSelection,
    isCreating,
    isSubmitDisabled,
  };
}

function NewSessionForm(props: NewSessionFormProps) {
  const { isMobile } = useResponsiveBreakpoint();
  const form = useSessionFormController(props);

  return (
    <form
      onSubmit={form.handleSubmit}
      className={isMobile ? "flex h-full min-h-0 flex-col overflow-hidden" : "min-w-0 space-y-4"}
    >
      <SessionFormFields
        {...form}
        isMobile={isMobile}
        taskId={props.taskId}
        workspaceId={props.workspaceId}
        executorLabel={props.executorLabel}
        executorProfile={props.executorProfile}
        worktreeBranch={props.worktreeBranch}
        initialPrompt={props.initialPrompt}
      />
      <SessionFormFooter
        isMobile={isMobile}
        isCreating={form.isCreating}
        isSubmitDisabled={form.isSubmitDisabled}
        onClose={props.onClose}
      />
    </form>
  );
}

function NoAgentBanner({
  noCompatibleProfiles,
  hasProfiles,
  executorProfileName,
}: {
  noCompatibleProfiles: boolean;
  hasProfiles: boolean;
  executorProfileName: string | null;
}) {
  const { t } = useTranslation();
  if (noCompatibleProfiles) {
    return (
      <p className="text-xs text-center text-muted-foreground">
        <Trans i18nKey="task:noAgentProfileConfiguredFor" values={{ name: executorProfileName }}>
          No agent profile is configured for{" "}
          <span className="text-foreground">“{executorProfileName}”</span>. Configure credentials in
          Settings → Executors.
        </Trans>
      </p>
    );
  }
  if (!hasProfiles) {
    return (
      <p className="text-xs text-center text-muted-foreground">
        {t("task:noAgentProfilesConfiguredAddOne")}
      </p>
    );
  }
  return null;
}

function createForkPreviewContext(
  conversationFork: ConversationForkFormContext | undefined,
  previewTriggerRef: { current: HTMLElement | null },
  setPreviewOpen: (open: boolean) => void,
) {
  if (!conversationFork) return undefined;
  return {
    ...conversationFork,
    onPreview: () => {
      previewTriggerRef.current =
        document.activeElement instanceof HTMLElement ? document.activeElement : null;
      setPreviewOpen(true);
    },
  };
}

export function NewSessionDialog({
  open,
  onOpenChange,
  taskId,
  workspaceId,
  groupId,
  handoff,
  conversationFork,
}: NewSessionDialogProps) {
  const { isMobile } = useResponsiveBreakpoint();
  const [previewOpen, setPreviewOpen] = useState(false);
  const previewTriggerRef = useRef<HTMLElement | null>(null);
  const {
    taskTitle,
    resolvedWorkspaceId,
    agentProfiles,
    currentSession,
    worktreeBranch,
    initialPrompt,
    executorLabel,
    sessionProfileId,
  } = useNewSessionDialogState(taskId);
  const executorProfile = useTaskExecutorProfile(taskId, open);
  const handoffLabel = handoffProfileLabel(agentProfiles, handoff);
  const formKey = handoff
    ? `${taskId}-${open}-${handoff.sourceSessionId}-${handoff.targetProfileId}`
    : `${taskId}-${open}`;
  const forkContext = createForkPreviewContext(conversationFork, previewTriggerRef, setPreviewOpen);
  const mobileTitleRef = useRef<HTMLHeadingElement>(null);
  const closePreview = useCallback(() => {
    setPreviewOpen(false);
    requestAnimationFrame(() => previewTriggerRef.current?.focus());
  }, []);
  useEffect(() => {
    if (!open) setPreviewOpen(false);
  }, [open]);
  const title = handoffLabel ? (
    <Trans i18nKey="task:handOffToTarget" values={{ label: handoffLabel }}>
      Hand off to <span className="text-foreground">{handoffLabel}</span>
    </Trans>
  ) : (
    <Trans i18nKey="task:newAgentInTask" values={{ title: taskTitle }}>
      New agent in <span className="text-foreground">{taskTitle}</span>
    </Trans>
  );
  const titleHeader = previewOpen ? null : (
    <DialogHeader className="shrink-0 px-4 pt-4 sm:px-0 sm:pt-0">
      <DialogTitle
        ref={mobileTitleRef}
        tabIndex={-1}
        className="min-w-0 wrap-break-word pr-6 text-sm font-medium"
      >
        {title}
      </DialogTitle>
    </DialogHeader>
  );
  const body = (
    <>
      {titleHeader}
      <div hidden={previewOpen} className={isMobile ? "min-h-0 flex-1 overflow-hidden" : ""}>
        <NewSessionForm
          key={formKey}
          taskId={taskId}
          workspaceId={workspaceId ?? resolvedWorkspaceId}
          currentProfileId={sessionProfileId}
          executorId={currentSession?.executor_id ?? ""}
          executorLabel={executorLabel}
          executorProfile={executorProfile}
          worktreeBranch={worktreeBranch}
          initialPrompt={initialPrompt}
          agentProfiles={agentProfiles}
          groupId={groupId}
          handoff={handoff}
          conversationFork={forkContext}
          onClose={() => onOpenChange(false)}
        />
      </div>
      {previewOpen && forkContext && (
        <ConversationForkPreview fork={forkContext} onBack={closePreview} />
      )}
    </>
  );

  const surfaceOnOpenChange = (next: boolean) => {
    if (isMobile && !next) setPreviewOpen(false);
    onOpenChange(next);
  };
  return (
    <NewSessionDialogSurface
      open={open}
      onOpenChange={surfaceOnOpenChange}
      isMobile={isMobile}
      previewOpen={previewOpen}
      onClosePreview={closePreview}
      mobileTitleRef={mobileTitleRef}
      hasConversationFork={!!conversationFork}
    >
      {body}
    </NewSessionDialogSurface>
  );
}
