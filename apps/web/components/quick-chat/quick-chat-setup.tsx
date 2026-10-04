"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconLoader2, IconSend2 } from "@tabler/icons-react";
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
import { WorkspaceRepoChips } from "@/components/task-create-dialog-workspace-repo-chips";
import { QuickChatRepositoryAction } from "./quick-chat-repository-action";
import { QuickChatMobileRepositoryChip } from "./quick-chat-mobile-repository-chip";
import { ConfigurationChatAction } from "./configuration-chat-action";
import { generateUUID } from "@/lib/utils";
import type { QuickChatSetupDraft } from "./use-quick-chat-setup-draft";
import { QuickChatAgentField, type QuickChatSetupRepositoryState } from "./quick-chat-setup-fields";
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
  const usesTouchDrawer = useTouchDrawer();
  const isMobile = useResponsiveBreakpoint().isMobile;
  const usesTouchTarget = usesTouchDrawer || isMobile;
  return (
    <Button
      type="button"
      size="icon"
      onClick={onClick}
      disabled={disabled}
      aria-label={busy ? t("chat:startingChat") : t("task:sendInput")}
      data-testid="quick-chat-send"
      data-dialog-default-action
      className={`cursor-pointer ${usesTouchTarget ? "h-11 w-11" : "h-7 w-7"}`}
    >
      {busy ? (
        <IconLoader2 className="h-4 w-4 animate-spin" aria-hidden />
      ) : (
        <IconSend2 className="h-4 w-4" aria-hidden />
      )}
    </Button>
  );
}

function QuickChatSetupError({ error }: { error: string | null }) {
  const { t } = useTranslation();
  if (!error) return null;
  return (
    <div
      className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive"
      data-testid="quick-chat-setup-error"
      role="alert"
    >
      <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
      <div className="min-w-0 space-y-1 break-words">
        <p className="font-medium">{t("chat:failedToStartQuickChat")}</p>
        <p className="text-muted-foreground">{error}</p>
      </div>
    </div>
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
    (repositoryId: string) =>
      updateRepositories((rows) =>
        rows.some((row) => row.repositoryId === repositoryId)
          ? rows
          : [...rows, { key: generateUUID(), repositoryId, branch: "" }],
      ),
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

function QuickChatAddRepositoryButton({
  kind,
  draft,
  repositories,
  isStarting,
}: Pick<QuickChatSetupLayoutProps, "kind" | "draft" | "repositories" | "isStarting">) {
  return (
    <QuickChatRepositoryAction
      repositories={repositories.repositories}
      selectedIds={draft.repositories.flatMap((row) =>
        row.repositoryId ? [row.repositoryId] : [],
      )}
      disabled={isStarting || kind === "config" || !repositories.canAddMore}
      hint={repositories.addHint}
      onSelect={repositories.addRepository}
    />
  );
}

function QuickChatComposerContext({
  workspaceId,
  kind,
  draft,
  repositories,
}: Pick<QuickChatSetupLayoutProps, "workspaceId" | "kind" | "draft" | "repositories">) {
  const { t } = useTranslation();
  const touch = useTouchDrawer();
  const { isMobile } = useResponsiveBreakpoint();
  if (kind === "config") {
    return (
      <div role="status" className="text-xs font-medium text-muted-foreground">
        {t("chat:configurationChat")}
      </div>
    );
  }
  if (draft.repositories.length === 0) return null;
  return (
    <div className="contents" data-testid="quick-chat-repository-chips">
      {touch || isMobile ? (
        draft.repositories.map((row) => (
          <QuickChatMobileRepositoryChip
            key={row.key}
            row={row}
            workspaceId={workspaceId}
            repositories={repositories}
          />
        ))
      ) : (
        <WorkspaceRepoChips
          rows={draft.repositories}
          repositories={repositories.repositories}
          workspaceId={workspaceId}
          canAddMore={false}
          showAddButton={false}
          allowDuplicateRepositories={false}
          onAdd={() => {}}
          onRemove={repositories.removeRepository}
          onRowRepositoryChange={repositories.handleRepositoryChange}
          onRowBranchChange={repositories.handleBranchChange}
        />
      )}
    </div>
  );
}

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
            toolbarLeadingActions={
              <QuickChatAddRepositoryButton
                kind={kind}
                draft={draft}
                repositories={repositories}
                isStarting={isStarting}
              />
            }
            toolbarAttachmentActions={
              canCreateConfigurationChat && (
                <ConfigurationChatAction
                  checked={kind === "config"}
                  disabled={isStarting}
                  onCheckedChange={(checked) => onKindChange(checked ? "config" : "chat")}
                />
              )
            }
            contextLeadingContent={
              <QuickChatComposerContext
                workspaceId={workspaceId}
                kind={kind}
                draft={draft}
                repositories={repositories}
              />
            }
            toolbarActions={
              <QuickChatSendButton
                disabled={!canSubmit || isStarting}
                busy={isStarting}
                onClick={handleSend}
              />
            }
          />
          <QuickChatSetupError error={kind === "config" ? configurationError : quickChatError} />
          <QuickChatAgentField
            kind={kind}
            profiles={profiles}
            draft={draft}
            disabled={isStarting}
            onDraftChange={onDraftChange}
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
