"use client";

import { memo, useCallback, useState } from "react";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconX, IconRocket } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useToast } from "@/components/toast-provider";
import { startQuickChat } from "@/lib/api/domains/workspace-api";
import type { Repository } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { isSelectableAgentProfile } from "@/lib/state/slices/settings/types";
import { useTranslation } from "react-i18next";
import {
  orderAgentProfilesByRecentUse,
  recordAgentProfileRecentUseBestEffort,
} from "@/lib/agent-profile-recent-use";
import type { AgentProfileRecentUseRecord } from "@/lib/agent-profile-recent-use";

type QuickChatPickerDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
};

type FormState = {
  selectedRepoId: string;
  setSelectedRepoId: (id: string) => void;
  selectedAgentId: string;
  setSelectedAgentId: (id: string) => void;
  repositories: Repository[];
  agentProfiles: AgentProfileOption[];
  recentProfileIds?: string[];
};

const NONE_VALUE = "__none__";

function recordQuickChatProfileUse(
  sessionId: string | undefined,
  profileId: string | undefined,
  onSuccess: (record: AgentProfileRecentUseRecord) => void,
) {
  if (!sessionId || !profileId) return;
  recordAgentProfileRecentUseBestEffort("quick_chat", profileId, onSuccess);
}

function QuickChatFormBody({ state }: { state: FormState }) {
  const { t } = useTranslation();
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  const { selectedRepoId, setSelectedRepoId, selectedAgentId, setSelectedAgentId } = state;
  const selectableProfiles = state.agentProfiles.filter((profile) =>
    isSelectableAgentProfile(profile, dynamicRoutingEnabled),
  );
  const orderedProfiles = orderAgentProfilesByRecentUse(
    selectableProfiles,
    state.recentProfileIds,
    selectedAgentId,
  );
  return (
    <div className="p-4 space-y-4">
      <p className="text-sm text-muted-foreground">{t("chat:quickChatDialogIntro")}</p>
      <div className="space-y-2">
        <Label htmlFor="repository">{t("chat:repositoryOptional")}</Label>
        <Select
          value={selectedRepoId || NONE_VALUE}
          onValueChange={(v) => setSelectedRepoId(v === NONE_VALUE ? "" : v)}
        >
          <SelectTrigger id="repository" className="w-full">
            <SelectValue placeholder={t("chat:selectARepository")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NONE_VALUE}>{t("chat:noRepository")}</SelectItem>
            {state.repositories.map((repo) => (
              <SelectItem key={repo.id} value={repo.id}>
                {repo.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-2">
        <Label htmlFor="agent">{t("chat:agentOptional")}</Label>
        <Select
          value={selectedAgentId || NONE_VALUE}
          onValueChange={(v) => setSelectedAgentId(v === NONE_VALUE ? "" : v)}
        >
          <SelectTrigger id="agent" className="w-full">
            <SelectValue placeholder={t("chat:useWorkspaceDefault")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NONE_VALUE}>{t("chat:useWorkspaceDefaultOption")}</SelectItem>
            {orderedProfiles.map((profile) => (
              <SelectItem key={profile.id} value={profile.id}>
                {profile.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}

/** Dialog to pick repository and agent for a new quick chat session */
export const QuickChatPickerDialog = memo(function QuickChatPickerDialog({
  open,
  onOpenChange,
  workspaceId,
}: QuickChatPickerDialogProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const openQuickChat = useAppStore((s) => s.openQuickChat);
  const applyAgentProfileRecentUse = useAppStore((s) => s.applyAgentProfileRecentUse);
  const agentGeneratedTaskTitles = useAppStore((s) => s.userSettings.agentGeneratedTaskTitles);
  const [isStarting, setIsStarting] = useState(false);
  const [selectedRepoId, setSelectedRepoId] = useState<string>("");
  const [selectedAgentId, setSelectedAgentId] = useState<string>("");
  const repositories = useAppStore((s) => s.repositories.itemsByWorkspaceId?.[workspaceId] ?? []);
  const agentProfiles = useAppStore((s) => s.agentProfiles.items ?? []);
  const recentProfileIds = useAppStore(
    (s) => s.agentProfileRecentUse?.records.quick_chat?.profileIds,
  );

  const handleStart = useCallback(async () => {
    if (isStarting) return;
    setIsStarting(true);
    try {
      const response = await startQuickChat(workspaceId, {
        repository_id: selectedRepoId || undefined,
        agent_profile_id: selectedAgentId || undefined,
        ...(agentGeneratedTaskTitles ? { auto_title: true } : {}),
      });
      const effectiveProfileId = response.agent_profile_id ?? (selectedAgentId || undefined);
      recordQuickChatProfileUse(response.session_id, effectiveProfileId, (record) =>
        applyAgentProfileRecentUse("quick_chat", record),
      );
      onOpenChange(false);
      // Open the quick chat modal with the new session
      openQuickChat(response.session_id, workspaceId, effectiveProfileId, "chat", response.task_id);
    } catch (error) {
      toast({
        title: t("chat:failedToStartQuickChat"),
        description: error instanceof Error ? error.message : t("chat:unknownError"),
        variant: "error",
      });
    } finally {
      setIsStarting(false);
    }
  }, [
    workspaceId,
    selectedRepoId,
    selectedAgentId,
    agentGeneratedTaskTitles,
    isStarting,
    onOpenChange,
    openQuickChat,
    applyAgentProfileRecentUse,
    toast,
  ]);

  const formState: FormState = {
    selectedRepoId,
    setSelectedRepoId,
    selectedAgentId,
    setSelectedAgentId,
    repositories,
    agentProfiles,
    recentProfileIds,
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="!max-w-md p-0 gap-0 flex flex-col shadow-2xl"
        showCloseButton={false}
        overlayClassName="bg-black/20"
      >
        <DialogTitle className="sr-only">{t("chat:newQuickChat")}</DialogTitle>
        <div className="flex items-center justify-between px-4 py-3 border-b">
          <h2 className="text-lg font-semibold">{t("chat:newQuickChat")}</h2>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onOpenChange(false)}
            className="cursor-pointer h-8 w-8"
          >
            <IconX className="h-4 w-4" />
          </Button>
        </div>
        <QuickChatFormBody state={formState} />
        <div className="flex justify-end gap-2 px-4 py-3 border-t bg-muted/30">
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            {t("common:cancel")}
          </Button>
          <Button onClick={handleStart} disabled={isStarting} className="cursor-pointer">
            <IconRocket className="h-4 w-4 mr-2" />
            {isStarting ? t("chat:starting") : t("chat:startChat")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
});

/** Alias for backwards compatibility */
export const QuickChatDialog = QuickChatPickerDialog;
