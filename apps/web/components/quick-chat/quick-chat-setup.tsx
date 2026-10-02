"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { IconLoader2, IconSend2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { QuickChatRepositoryInput } from "@/lib/api/domains/workspace-api";
import type { AgentProfileOption } from "@/lib/state/slices";
import { isSelectableAgentProfile } from "@/lib/state/slices/settings/types";
import type { QuickChatOpeningPayload, QuickChatSessionKind } from "@/lib/state/slices/ui/types";
import type { TaskRepoRow, TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import type { FileAttachment } from "@/components/task/chat/file-attachment";
import { TaskFormInputs } from "@/components/task-create-dialog-selectors";
import { generateUUID } from "@/lib/utils";
import type { QuickChatSetupDraft } from "./use-quick-chat-setup-draft";
import {
  QuickChatSetupSelectionFields,
  type QuickChatSetupRepositoryState,
} from "./quick-chat-setup-fields";
import {
  buildOpeningPayload,
  hasUnavailableAttachment,
  isInvalidOpeningRequest,
  toQuickChatRepositoryInputs,
} from "./quick-chat-setup-utils";

type QuickChatSetupProps = {
  workspaceId: string;
  kind: QuickChatSessionKind;
  canCreateConfigurationChat: boolean;
  defaultConfigProfileId?: string;
  pendingAgentId: string | null;
  configurationStarting: boolean;
  configurationError: string | null;
  quickChatError: string | null;
  draft: QuickChatSetupDraft;
  onDraftChange: (patch: Partial<QuickChatSetupDraft>) => boolean | void;
  onStartQuickChat: (
    agentId: string,
    repositories: QuickChatRepositoryInput[],
    payload: QuickChatOpeningPayload,
  ) => Promise<boolean>;
  onStartConfigChat: (agentId: string, payload: QuickChatOpeningPayload) => Promise<boolean>;
  onKindChange: (kind: QuickChatSessionKind) => void;
  onDiscardDraft: () => void;
  onRegisterDiscard: (discard: () => void) => () => void;
};

function repositoryAddState(
  t: ReturnType<typeof useTranslation>["t"],
  isLoading: boolean,
  repositoryCount: number,
  rowCount: number,
) {
  if (isLoading) return { canAddMore: false, addHint: t("chat:loadingRepositories") };
  if (repositoryCount === 0) {
    return { canAddMore: false, addHint: t("chat:noRepositoriesAvailableInWorkspace") };
  }
  if (rowCount >= repositoryCount) {
    return { canAddMore: false, addHint: t("chat:allWorkspaceRepositoriesAdded") };
  }
  return { canAddMore: true, addHint: undefined };
}

function QuickChatSendButton({
  disabled,
  busy,
  onClick,
}: {
  disabled: boolean;
  busy: boolean;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer() || useResponsiveBreakpoint().isMobile;
  return (
    <Button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={busy ? t("chat:startingChat") : t("task:sendInput")}
      data-testid="quick-chat-send"
      data-dialog-default-action
      className={`cursor-pointer ${usesTouchDrawer ? "h-12 w-12" : "h-9 w-9"}`}
    >
      {busy ? (
        <IconLoader2 className="h-4 w-4 animate-spin" aria-hidden />
      ) : (
        <IconSend2 className="h-4 w-4" aria-hidden />
      )}
    </Button>
  );
}

type QuickChatSetupProfileState = {
  profiles: AgentProfileOption[];
  selectableDefault: string;
  selectedProfileEnabled: boolean;
};

function useQuickChatSetupProfile(
  workspaceId: string,
  kind: QuickChatSessionKind,
  defaultConfigProfileId: string | undefined,
  draft: QuickChatSetupDraft,
  onDraftChange: QuickChatSetupProps["onDraftChange"],
): QuickChatSetupProfileState {
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  const profiles = useAppStore((state) => state.agentProfiles.items ?? []);
  const workspaceDefaultAgentId = useAppStore(
    (state) =>
      state.workspaces.items.find((workspace) => workspace.id === workspaceId)
        ?.default_agent_profile_id ?? "",
  );
  const isSelectable = (profileId: string | undefined) =>
    Boolean(
      profileId &&
      profiles.some(
        (profile) =>
          profile.id === profileId && isSelectableAgentProfile(profile, dynamicRoutingEnabled),
      ),
    );
  let selectableDefault = "";
  if (kind === "config") {
    selectableDefault = [defaultConfigProfileId, workspaceDefaultAgentId].find(isSelectable) ?? "";
  } else if (isSelectable(workspaceDefaultAgentId)) {
    selectableDefault = workspaceDefaultAgentId;
  }
  useEffect(() => {
    if (!draft.agentProfileExplicit && draft.agentProfileId !== selectableDefault) {
      onDraftChange({ agentProfileId: selectableDefault });
    }
  }, [draft.agentProfileExplicit, draft.agentProfileId, onDraftChange, selectableDefault]);
  const selectedProfileEnabled = profiles.some(
    (profile) =>
      profile.id === draft.agentProfileId &&
      isSelectableAgentProfile(profile, dynamicRoutingEnabled),
  );
  return { profiles, selectableDefault, selectedProfileEnabled };
}

function useQuickChatSetupRepositories(
  workspaceId: string,
  kind: QuickChatSessionKind,
  draft: QuickChatSetupDraft,
  onDraftChange: QuickChatSetupProps["onDraftChange"],
): QuickChatSetupRepositoryState {
  const { t } = useTranslation();
  const { repositories, isLoading } = useRepositories(workspaceId, true);
  const selectedRepositories = useMemo<QuickChatRepositoryInput[]>(
    () => toQuickChatRepositoryInputs(draft.repositories),
    [draft.repositories],
  );
  const updateRepositories = useCallback(
    (update: (rows: TaskRepoRow[]) => TaskRepoRow[]) =>
      onDraftChange({ repositories: update(draft.repositories) }),
    [draft.repositories, onDraftChange],
  );
  const handleRepositoryChange = useCallback(
    (key: string, repositoryId: string) =>
      updateRepositories((rows) =>
        rows.map((row) =>
          row.key === key ? { ...row, repositoryId, localPath: undefined, branch: "" } : row,
        ),
      ),
    [updateRepositories],
  );
  const handleBranchChange = useCallback(
    (key: string, branch: string) =>
      updateRepositories((rows) => rows.map((row) => (row.key === key ? { ...row, branch } : row))),
    [updateRepositories],
  );
  const addRepository = useCallback(
    () => updateRepositories((rows) => [...rows, { key: generateUUID(), branch: "" }]),
    [updateRepositories],
  );
  const removeRepository = useCallback(
    (key: string) => updateRepositories((rows) => rows.filter((row) => row.key !== key)),
    [updateRepositories],
  );
  const { canAddMore, addHint } = repositoryAddState(
    t,
    isLoading,
    repositories.length,
    draft.repositories.length,
  );
  return {
    repositories,
    canAddMore,
    addHint,
    selectedRepositories,
    hasIncompleteRepository:
      kind === "chat" && draft.repositories.some((row) => !row.repositoryId || !row.branch),
    addRepository,
    removeRepository,
    handleRepositoryChange,
    handleBranchChange,
  };
}

function useQuickChatSetupActions(args: {
  kind: QuickChatSessionKind;
  draft: QuickChatSetupDraft;
  selectedRepositories: QuickChatRepositoryInput[];
  profileEnabled: boolean;
  isStarting: boolean;
  onStartQuickChat: QuickChatSetupProps["onStartQuickChat"];
  onStartConfigChat: QuickChatSetupProps["onStartConfigChat"];
}) {
  const formRef = useRef<TaskFormInputsHandle>(null);
  const submitLock = useRef(false);
  const chatSubmitKey = useAppStore((state) => state.userSettings.chatSubmitKey);
  const composerSubmitDisabled =
    args.isStarting ||
    !args.profileEnabled ||
    hasUnavailableAttachment(args.draft.attachments) ||
    (args.kind === "chat" &&
      args.draft.repositories.some((row) => !row.repositoryId || !row.branch));
  const promptReady = (formRef.current?.getValue() ?? args.draft.message).trim().length > 0;
  const canSubmit = promptReady && !composerSubmitDisabled;

  const handleSend = useCallback(async () => {
    if (submitLock.current) return false;
    const message = formRef.current?.getValue() ?? args.draft.message;
    const attachments = formRef.current?.getAttachments() ?? args.draft.attachments;
    if (
      isInvalidOpeningRequest({
        message,
        attachments,
        repositories: args.draft.repositories,
        kind: args.kind,
        isStarting: args.isStarting,
        profileEnabled: args.profileEnabled,
      })
    )
      return false;

    const payload = buildOpeningPayload(message, attachments);
    submitLock.current = true;
    const accepted =
      args.kind === "config"
        ? await args.onStartConfigChat(args.draft.agentProfileId, payload)
        : await args.onStartQuickChat(
            args.draft.agentProfileId,
            args.selectedRepositories,
            payload,
          );
    if (!accepted) submitLock.current = false;
    return accepted;
  }, [args]);

  const handlePromptKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      const hasSubmitModifier = event.metaKey || event.ctrlKey;
      if (
        event.key !== "Enter" ||
        event.shiftKey ||
        event.altKey ||
        event.repeat ||
        event.nativeEvent.isComposing ||
        event.keyCode === 229 ||
        (chatSubmitKey === "enter" ? hasSubmitModifier : !hasSubmitModifier)
      )
        return;
      event.preventDefault();
      void handleSend();
    },
    [chatSubmitKey, handleSend],
  );

  return { formRef, composerSubmitDisabled, canSubmit, handleSend, handlePromptKeyDown };
}

type QuickChatSetupLayoutProps = {
  workspaceId: string;
  kind: QuickChatSessionKind;
  canCreateConfigurationChat: boolean;
  configurationError: string | null;
  quickChatError: string | null;
  draft: QuickChatSetupDraft;
  profiles: AgentProfileOption[];
  repositories: QuickChatSetupRepositoryState;
  formRef: React.RefObject<TaskFormInputsHandle | null>;
  isStarting: boolean;
  usesTouchDrawer: boolean;
  composerSubmitDisabled: boolean;
  canSubmit: boolean;
  onDraftChange: QuickChatSetupProps["onDraftChange"];
  onKindChange: QuickChatSetupProps["onKindChange"];
  handleSend: () => Promise<boolean>;
  handlePromptKeyDown: (event: React.KeyboardEvent) => void;
};

function QuickChatSetupLayout({
  workspaceId,
  kind,
  canCreateConfigurationChat,
  configurationError,
  quickChatError,
  draft,
  profiles,
  repositories,
  formRef,
  isStarting,
  usesTouchDrawer,
  composerSubmitDisabled,
  canSubmit,
  onDraftChange,
  onKindChange,
  handleSend,
  handlePromptKeyDown,
}: QuickChatSetupLayoutProps) {
  const { t } = useTranslation();
  const handleDescriptionValueChange = useCallback(
    (message: string) => onDraftChange({ message }),
    [onDraftChange],
  );
  const handleAttachmentsChange = useCallback(
    (attachments: FileAttachment[]) => onDraftChange({ attachments }),
    [onDraftChange],
  );
  return (
    <div className="flex min-h-0 flex-1 flex-col bg-popover" data-testid="quick-chat-setup">
      <div
        className="min-h-0 flex-1 overflow-y-auto px-4 py-5 sm:px-8 sm:py-8"
        data-testid="quick-chat-setup-scroll"
      >
        <div className="mx-auto flex min-h-full w-full max-w-2xl flex-col justify-center gap-5">
          <h2 className="text-lg font-semibold" data-testid="quick-chat-introduction">
            {t("chat:quickChatIntro")}
          </h2>
          <TaskFormInputs
            workspaceId={workspaceId}
            isSessionMode
            quickChatComposer
            autoFocus={!usesTouchDrawer}
            initialDescription={draft.message}
            initialAttachments={draft.attachments}
            onDescriptionChange={() => {}}
            onDescriptionValueChange={handleDescriptionValueChange}
            onAttachmentsChange={handleAttachmentsChange}
            onKeyDown={handlePromptKeyDown}
            descriptionValueRef={formRef}
            disabled={isStarting}
            composerSubmitDisabled={composerSubmitDisabled}
            placeholder={t("task:writeAPromptForTheAgent")}
            onComposerSubmit={handleSend}
            toolbarActions={
              <QuickChatSendButton
                disabled={!canSubmit || isStarting}
                busy={isStarting}
                onClick={handleSend}
              />
            }
          />

          {(kind === "config" ? configurationError : quickChatError) && (
            <p role="alert" className="text-sm text-destructive">
              {kind === "config" ? configurationError : quickChatError}
            </p>
          )}

          <QuickChatSetupSelectionFields
            workspaceId={workspaceId}
            kind={kind}
            canCreateConfigurationChat={canCreateConfigurationChat}
            draft={draft}
            profiles={profiles}
            repositories={repositories}
            isStarting={isStarting}
            onDraftChange={onDraftChange}
            onKindChange={onKindChange}
          />
        </div>
      </div>
    </div>
  );
}

export function QuickChatSetup(props: QuickChatSetupProps) {
  const { workspaceId, kind, defaultConfigProfileId, pendingAgentId, configurationStarting } =
    props;
  const usesTouchDrawer = useTouchDrawer();
  const isStarting = pendingAgentId !== null || configurationStarting;
  const profile = useQuickChatSetupProfile(
    workspaceId,
    kind,
    defaultConfigProfileId,
    props.draft,
    props.onDraftChange,
  );
  const repositories = useQuickChatSetupRepositories(
    workspaceId,
    kind,
    props.draft,
    props.onDraftChange,
  );
  const actions = useQuickChatSetupActions({
    kind,
    draft: props.draft,
    selectedRepositories: repositories.selectedRepositories,
    profileEnabled: profile.selectedProfileEnabled,
    isStarting,
    onStartQuickChat: props.onStartQuickChat,
    onStartConfigChat: props.onStartConfigChat,
  });
  const handleDiscard = useCallback(() => {
    actions.formRef.current?.clearAttachments?.();
    props.onDiscardDraft();
  }, [actions.formRef, props.onDiscardDraft]);
  useEffect(() => props.onRegisterDiscard(handleDiscard), [handleDiscard, props.onRegisterDiscard]);

  return (
    <QuickChatSetupLayout
      workspaceId={workspaceId}
      kind={kind}
      canCreateConfigurationChat={props.canCreateConfigurationChat}
      configurationError={props.configurationError}
      quickChatError={props.quickChatError}
      draft={props.draft}
      profiles={profile.profiles}
      repositories={repositories}
      formRef={actions.formRef}
      isStarting={isStarting}
      usesTouchDrawer={usesTouchDrawer}
      composerSubmitDisabled={actions.composerSubmitDisabled}
      canSubmit={actions.canSubmit}
      onDraftChange={props.onDraftChange}
      onKindChange={props.onKindChange}
      handleSend={actions.handleSend}
      handlePromptKeyDown={actions.handlePromptKeyDown}
    />
  );
}
