"use client";

import { useTranslation } from "react-i18next";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { Repository } from "@/lib/types/http";
import type { QuickChatSessionKind } from "@/lib/state/slices/ui/types";
import type { QuickChatSetupDraft } from "./use-quick-chat-setup-draft";
import { QuickChatAgentPicker } from "./quick-chat-agent-picker";

export type QuickChatSetupRepositoryState = {
  repositories: Repository[];
  canAddMore: boolean;
  addHint?: string;
  selectedRepositories: { repository_id: string; base_branch: string }[];
  hasIncompleteRepository: boolean;
  addRepository: (repositoryId: string) => void;
  removeRepository: (key: string) => void;
  handleRepositoryChange: (key: string, repositoryId: string) => void;
  handleBranchChange: (key: string, branch: string) => void;
};

export function QuickChatAgentField({
  profiles,
  kind,
  draft,
  disabled,
  onDraftChange,
}: {
  profiles: AgentProfileOption[];
  kind: QuickChatSessionKind;
  draft: QuickChatSetupDraft;
  disabled: boolean;
  onDraftChange: (patch: Partial<QuickChatSetupDraft>) => boolean | void;
}) {
  const { t } = useTranslation();
  const options = useAgentProfileOptions(
    profiles,
    kind === "config" ? "config_chat" : "quick_chat",
  );
  return (
    <section className="min-w-0 space-y-2" aria-labelledby="quick-chat-agent-label">
      <div className="sr-only">
        <h3 id="quick-chat-agent-label" className="text-sm font-medium">
          {t("chat:agentProfile")}
        </h3>
        <p id="quick-chat-agent-help" className="text-xs text-muted-foreground">
          {t("chat:agentProfileHelp")}
        </p>
      </div>
      <QuickChatAgentPicker
        options={options}
        value={draft.agentProfileId}
        onValueChange={(agentProfileId) =>
          onDraftChange({ agentProfileId, agentProfileExplicit: true })
        }
        disabled={disabled}
        placeholder={profiles.length > 0 ? t("chat:selectAgent") : t("chat:noAgentsAvailable")}
        labelId="quick-chat-agent-label"
        ariaDescribedBy="quick-chat-agent-help"
      />
    </section>
  );
}
