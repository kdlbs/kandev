"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconInfoCircle } from "@tabler/icons-react";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { WorkspaceRepoChips } from "@/components/task-create-dialog-workspace-repo-chips";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { Repository } from "@/lib/types/http";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import type { QuickChatSessionKind } from "@/lib/state/slices/ui/types";
import type { QuickChatSetupDraft } from "./use-quick-chat-setup-draft";
import { QuickChatAgentPicker } from "./quick-chat-agent-picker";
import { ConfigurationChatToggle } from "./configuration-chat-toggle";

export type QuickChatSetupRepositoryState = {
  repositories: Repository[];
  canAddMore: boolean;
  addHint?: string;
  selectedRepositories: { repository_id: string; base_branch: string }[];
  hasIncompleteRepository: boolean;
  addRepository: () => void;
  removeRepository: (key: string) => void;
  handleRepositoryChange: (key: string, repositoryId: string) => void;
  handleBranchChange: (key: string, branch: string) => void;
};

type QuickChatSetupSelectionFieldsProps = {
  workspaceId: string;
  kind: QuickChatSessionKind;
  canCreateConfigurationChat: boolean;
  draft: QuickChatSetupDraft;
  profiles: AgentProfileOption[];
  repositories: QuickChatSetupRepositoryState;
  isStarting: boolean;
  onDraftChange: (patch: Partial<QuickChatSetupDraft>) => void;
  onKindChange: (kind: QuickChatSessionKind) => void;
};

function AgentField({
  profiles,
  kind,
  value,
  disabled,
  placeholder,
  onChange,
}: {
  profiles: AgentProfileOption[];
  kind: QuickChatSessionKind;
  value: string;
  disabled: boolean;
  placeholder: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const options = useAgentProfileOptions(
    profiles,
    kind === "config" ? "config_chat" : "quick_chat",
  );
  return (
    <section className="min-w-0 flex-1 space-y-2" aria-labelledby="quick-chat-agent-label">
      <div>
        <h3 id="quick-chat-agent-label" className="text-sm font-medium">
          {t("chat:agentProfile")}
        </h3>
        <p id="quick-chat-agent-help" className="text-xs text-muted-foreground">
          {t("chat:agentProfileHelp")}
        </p>
      </div>
      <QuickChatAgentPicker
        options={options}
        value={value}
        onValueChange={onChange}
        disabled={disabled}
        placeholder={placeholder}
        labelId="quick-chat-agent-label"
        ariaDescribedBy="quick-chat-agent-help"
      />
    </section>
  );
}

function RepositoryContextHelp() {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const isMobile = useResponsiveBreakpoint().isMobile;
  const usesTouchDrawerOnCurrentDevice = usesTouchDrawer || isMobile;
  const [open, setOpen] = useState(false);
  const trigger = (
    <button
      type="button"
      aria-label={t("chat:aboutRepositoryContext")}
      className={`flex shrink-0 cursor-pointer items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground ${usesTouchDrawerOnCurrentDevice ? "h-11 w-11" : "h-7 w-7"}`}
      onClick={() => usesTouchDrawerOnCurrentDevice && setOpen(true)}
    >
      <IconInfoCircle className="h-4 w-4" />
    </button>
  );
  if (usesTouchDrawerOnCurrentDevice) {
    return (
      <>
        {trigger}
        <Drawer open={open} onOpenChange={setOpen}>
          <DrawerContent>
            <DrawerHeader>
              <DrawerTitle>{t("chat:aboutRepositoryContext")}</DrawerTitle>
              <DrawerDescription>{t("chat:repositoryContextHelp")}</DrawerDescription>
            </DrawerHeader>
          </DrawerContent>
        </Drawer>
      </>
    );
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>{trigger}</TooltipTrigger>
      <TooltipContent className="max-w-xs">{t("chat:repositoryContextHelp")}</TooltipContent>
    </Tooltip>
  );
}

function RepositoryField({
  workspaceId,
  repositories,
  rows,
  canAddMore,
  addHint,
  onAdd,
  onRemove,
  onRepositoryChange,
  onBranchChange,
}: {
  workspaceId: string;
  repositories: Repository[];
  rows: TaskRepoRow[];
  canAddMore: boolean;
  addHint?: string;
  onAdd: () => void;
  onRemove: (key: string) => void;
  onRepositoryChange: (key: string, value: string) => void;
  onBranchChange: (key: string, value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <section className="min-w-0 flex-1 space-y-2" aria-labelledby="quick-chat-repositories-label">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <h3 id="quick-chat-repositories-label" className="text-sm font-medium">
            {t("chat:repositories")}{" "}
            <span className="font-normal text-muted-foreground">{t("chat:optional")}</span>
          </h3>
          <p id="quick-chat-repositories-help" className="text-xs text-muted-foreground">
            {t("chat:repositoriesHelp")}
          </p>
        </div>
        <RepositoryContextHelp />
      </div>
      <div className="flex min-h-11 flex-wrap items-center gap-2">
        <WorkspaceRepoChips
          rows={rows}
          repositories={repositories}
          workspaceId={workspaceId}
          canAddMore={canAddMore}
          addHint={addHint}
          addLabel={t("chat:addRepository")}
          allowDuplicateRepositories={false}
          ariaDescribedBy="quick-chat-repositories-help"
          onAdd={onAdd}
          onRemove={onRemove}
          onRowRepositoryChange={onRepositoryChange}
          onRowBranchChange={onBranchChange}
        />
      </div>
    </section>
  );
}

export function QuickChatSetupSelectionFields({
  workspaceId,
  kind,
  canCreateConfigurationChat,
  draft,
  profiles,
  repositories,
  isStarting,
  onDraftChange,
  onKindChange,
}: QuickChatSetupSelectionFieldsProps) {
  const { t } = useTranslation();
  return (
    <>
      <div className="flex min-w-0 flex-col gap-4 md:flex-row md:items-end">
        <AgentField
          profiles={profiles}
          kind={kind}
          value={draft.agentProfileId}
          disabled={isStarting}
          placeholder={profiles.length > 0 ? t("chat:selectAgent") : t("chat:noAgentsAvailable")}
          onChange={(agentProfileId) =>
            onDraftChange({ agentProfileId, agentProfileExplicit: true })
          }
        />
        {kind === "chat" && (
          <RepositoryField
            workspaceId={workspaceId}
            repositories={repositories.repositories}
            rows={draft.repositories}
            canAddMore={repositories.canAddMore}
            addHint={repositories.addHint}
            onAdd={repositories.addRepository}
            onRemove={repositories.removeRepository}
            onRepositoryChange={repositories.handleRepositoryChange}
            onBranchChange={repositories.handleBranchChange}
          />
        )}
      </div>
      {canCreateConfigurationChat && (
        <ConfigurationChatToggle
          checked={kind === "config"}
          disabled={isStarting}
          onCheckedChange={(checked) => onKindChange(checked ? "config" : "chat")}
        />
      )}
    </>
  );
}
