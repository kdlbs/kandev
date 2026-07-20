"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { IconInfoCircle, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { AgentSelector } from "@/components/task-create-dialog-selectors";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";
import { WorkspaceRepoChips } from "@/components/task-create-dialog-workspace-repo-chips";
import { useRepositoriesState } from "@/components/task-create-dialog-repositories-state";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import type { QuickChatRepositoryInput } from "@/lib/api/domains/workspace-api";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { Repository } from "@/lib/types/http";
import type { QuickChatSessionKind } from "@/lib/state/slices/ui/types";
import { ConfigurationChatToggle } from "./configuration-chat-toggle";

type QuickChatSetupProps = {
  workspaceId: string;
  canCreateConfigurationChat: boolean;
  pendingAgentId: string | null;
  onStart: (agentId: string, repositories: QuickChatRepositoryInput[]) => void;
  onCancel: () => void;
  onKindChange: (kind: QuickChatSessionKind) => void;
};

function repositoryAddState(isLoading: boolean, repositoryCount: number, rowCount: number) {
  if (isLoading) return { canAddMore: false, addHint: "Loading repositories" };
  if (repositoryCount === 0) {
    return { canAddMore: false, addHint: "No repositories available in this workspace" };
  }
  if (rowCount >= repositoryCount) {
    return { canAddMore: false, addHint: "All workspace repositories are already added" };
  }
  return { canAddMore: true, addHint: undefined };
}

function QuickChatIntroduction() {
  return (
    <div className="space-y-1" data-testid="quick-chat-introduction">
      <p className="text-sm text-foreground">
        Chat with an agent about an idea, question, or codebase.
      </p>
      <p className="text-sm text-muted-foreground">Quick chats stay outside your task board.</p>
    </div>
  );
}

function AgentField({
  profiles,
  value,
  disabled,
  onChange,
}: {
  profiles: AgentProfileOption[];
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const options = useAgentProfileOptions(profiles);
  return (
    <section className="space-y-2" aria-labelledby="quick-chat-agent-label">
      <div>
        <h3 id="quick-chat-agent-label" className="text-sm font-medium">
          Agent profile
        </h3>
        <p id="quick-chat-agent-help" className="text-xs text-muted-foreground">
          Choose the agent for this conversation.
        </p>
      </div>
      <AgentSelector
        options={options}
        value={value}
        onValueChange={onChange}
        disabled={disabled}
        placeholder={profiles.length > 0 ? "Select agent" : "No agents available"}
        triggerClassName="h-11 w-full justify-between border border-input bg-background px-3 shadow-xs hover:bg-accent/50 data-[state=open]:border-ring data-[state=open]:ring-[2px] data-[state=open]:ring-ring/35"
        popoverPortal
      />
    </section>
  );
}

type RepositoryFieldProps = {
  workspaceId: string;
  repositories: Repository[];
  rows: ReturnType<typeof useRepositoriesState>["repositories"];
  canAddMore: boolean;
  addHint?: string;
  onAdd: () => void;
  onRemove: (key: string) => void;
  onRepositoryChange: (key: string, value: string) => void;
  onBranchChange: (key: string, value: string) => void;
};

function RepositoryField(props: RepositoryFieldProps) {
  return (
    <section className="space-y-3" aria-labelledby="quick-chat-repositories-label">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <h3 id="quick-chat-repositories-label" className="text-sm font-medium">
            Repositories <span className="font-normal text-muted-foreground">(optional)</span>
          </h3>
          <p id="quick-chat-repositories-help" className="text-xs text-muted-foreground">
            Add repository context to focus on specific code and branches.
          </p>
        </div>
        <RepositoryContextHelp />
      </div>
      <div
        className="flex min-h-11 flex-wrap items-center gap-2"
        aria-describedby="quick-chat-repositories-help"
      >
        <WorkspaceRepoChips
          rows={props.rows}
          repositories={props.repositories}
          workspaceId={props.workspaceId}
          canAddMore={props.canAddMore}
          addHint={props.addHint}
          addLabel="Add repository"
          allowDuplicateRepositories={false}
          onAdd={props.onAdd}
          onRemove={props.onRemove}
          onRowRepositoryChange={props.onRepositoryChange}
          onRowBranchChange={props.onBranchChange}
        />
      </div>
    </section>
  );
}

function RepositoryContextHelp() {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label="About repository context"
          className="flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <IconInfoCircle className="h-4 w-4" />
        </button>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">
        Kandev uses an isolated worktree from the selected branch. Uncommitted local changes are not
        included.
      </TooltipContent>
    </Tooltip>
  );
}

function SetupFooter({
  isStarting,
  startDisabled,
  onCancel,
  onStart,
}: {
  isStarting: boolean;
  startDisabled: boolean;
  onCancel: () => void;
  onStart: () => void;
}) {
  return (
    <footer
      className="flex shrink-0 items-center justify-end gap-2 border-t bg-popover px-4 py-3 sm:px-8"
      data-testid="quick-chat-setup-footer"
    >
      <Button variant="outline" onClick={onCancel} disabled={isStarting} className="cursor-pointer">
        Cancel
      </Button>
      <Button
        onClick={onStart}
        disabled={startDisabled}
        className="min-w-28 cursor-pointer"
        data-testid="quick-chat-start"
        data-dialog-default-action
      >
        {isStarting ? <IconLoader2 className="h-4 w-4 animate-spin" /> : null}
        {isStarting ? "Starting chat..." : "Start chat"}
      </Button>
    </footer>
  );
}

export function QuickChatSetup({
  workspaceId,
  canCreateConfigurationChat,
  pendingAgentId,
  onStart,
  onCancel,
  onKindChange,
}: QuickChatSetupProps) {
  const agentProfiles = useAppStore((state) => state.agentProfiles.items ?? []);
  const defaultAgentId = useAppStore(
    (state) =>
      state.workspaces.items.find((workspace) => workspace.id === workspaceId)
        ?.default_agent_profile_id ?? "",
  );
  const [agentProfileId, setAgentProfileId] = useState(defaultAgentId);
  useEffect(() => {
    if (!defaultAgentId) return;
    setAgentProfileId((current) => current || defaultAgentId);
  }, [defaultAgentId]);
  const { repositories, isLoading } = useRepositories(workspaceId, true);
  const repoState = useRepositoriesState();
  const isStarting = pendingAgentId !== null;
  const hasIncompleteRow = repoState.repositories.some((row) => !row.repositoryId || !row.branch);
  const { canAddMore, addHint } = repositoryAddState(
    isLoading,
    repositories.length,
    repoState.repositories.length,
  );
  const handleRepositoryChange = useCallback(
    (key: string, repositoryId: string) => {
      repoState.updateRepository(key, { repositoryId, localPath: undefined, branch: "" });
    },
    [repoState],
  );
  const handleBranchChange = useCallback(
    (key: string, branch: string) => repoState.updateRepository(key, { branch }),
    [repoState],
  );
  const selectedRepositories = useMemo<QuickChatRepositoryInput[]>(
    () =>
      repoState.repositories
        .filter((row) => row.repositoryId && row.branch)
        .map((row) => ({ repository_id: row.repositoryId as string, base_branch: row.branch })),
    [repoState.repositories],
  );
  const startDisabled = !agentProfileId || hasIncompleteRow || isStarting;
  return (
    <div className="flex min-h-0 flex-1 flex-col bg-popover" data-testid="quick-chat-setup">
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-6 sm:px-8 sm:py-8">
        <div className="mx-auto w-full max-w-2xl space-y-7">
          <header className="space-y-1">
            <h2 className="text-lg font-semibold">Quick Chat</h2>
            <QuickChatIntroduction />
          </header>
          {canCreateConfigurationChat && (
            <ConfigurationChatToggle
              checked={false}
              disabled={isStarting}
              onCheckedChange={(checked) => checked && onKindChange("config")}
            />
          )}
          <AgentField
            profiles={agentProfiles}
            value={agentProfileId}
            disabled={isStarting}
            onChange={setAgentProfileId}
          />
          <RepositoryField
            workspaceId={workspaceId}
            repositories={repositories}
            rows={repoState.repositories}
            canAddMore={canAddMore}
            addHint={addHint}
            onAdd={repoState.addRepository}
            onRemove={repoState.removeRepository}
            onRepositoryChange={handleRepositoryChange}
            onBranchChange={handleBranchChange}
          />
        </div>
      </div>
      <SetupFooter
        isStarting={isStarting}
        startDisabled={startDisabled}
        onCancel={onCancel}
        onStart={() => onStart(agentProfileId, selectedRepositories)}
      />
    </div>
  );
}
