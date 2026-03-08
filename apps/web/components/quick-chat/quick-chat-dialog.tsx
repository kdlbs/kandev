"use client";

import { memo, useCallback, useState } from "react";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconX, IconRocket } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { startQuickChat } from "@/lib/api/domains/workspace-api";
import type { Repository } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";

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
};

const NONE_VALUE = "__none__";

function QuickChatFormBody({ state }: { state: FormState }) {
  const { selectedRepoId, setSelectedRepoId, selectedAgentId, setSelectedAgentId } = state;
  return (
    <div className="p-4 space-y-4">
      <p className="text-sm text-muted-foreground">
        Start a quick conversation with an agent without creating a formal task.
      </p>
      <div className="space-y-2">
        <Label htmlFor="repository">Repository (optional)</Label>
        <Select
          value={selectedRepoId || NONE_VALUE}
          onValueChange={(v) => setSelectedRepoId(v === NONE_VALUE ? "" : v)}
        >
          <SelectTrigger id="repository" className="w-full">
            <SelectValue placeholder="Select a repository..." />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NONE_VALUE}>No repository</SelectItem>
            {state.repositories.map((repo) => (
              <SelectItem key={repo.id} value={repo.id}>
                {repo.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-2">
        <Label htmlFor="agent">Agent (optional)</Label>
        <Select
          value={selectedAgentId || NONE_VALUE}
          onValueChange={(v) => setSelectedAgentId(v === NONE_VALUE ? "" : v)}
        >
          <SelectTrigger id="agent" className="w-full">
            <SelectValue placeholder="Use workspace default..." />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NONE_VALUE}>Use workspace default</SelectItem>
            {state.agentProfiles.map((profile) => (
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
  const { toast } = useToast();
  const openQuickChat = useAppStore((s) => s.openQuickChat);
  const [isStarting, setIsStarting] = useState(false);
  const [selectedRepoId, setSelectedRepoId] = useState<string>("");
  const [selectedAgentId, setSelectedAgentId] = useState<string>("");
  const repositories = useAppStore((s) => s.repositories.itemsByWorkspaceId?.[workspaceId] ?? []);
  const agentProfiles = useAppStore((s) => s.agentProfiles.items ?? []);

  const handleStart = useCallback(async () => {
    if (isStarting) return;
    setIsStarting(true);
    try {
      const response = await startQuickChat(workspaceId, {
        repository_id: selectedRepoId || undefined,
        agent_profile_id: selectedAgentId || undefined,
      });
      onOpenChange(false);
      // Open the quick chat modal with the new session
      openQuickChat(response.session_id, workspaceId);
    } catch (error) {
      toast({
        title: "Failed to start quick chat",
        description: error instanceof Error ? error.message : "Unknown error",
        variant: "error",
      });
    } finally {
      setIsStarting(false);
    }
  }, [
    workspaceId,
    selectedRepoId,
    selectedAgentId,
    isStarting,
    onOpenChange,
    openQuickChat,
    toast,
  ]);

  const formState: FormState = {
    selectedRepoId,
    setSelectedRepoId,
    selectedAgentId,
    setSelectedAgentId,
    repositories,
    agentProfiles,
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="!max-w-md p-0 gap-0 flex flex-col shadow-2xl"
        showCloseButton={false}
        overlayClassName="bg-black/20"
      >
        <DialogTitle className="sr-only">New Quick Chat</DialogTitle>
        <div className="flex items-center justify-between px-4 py-3 border-b">
          <h2 className="text-lg font-semibold">New Quick Chat</h2>
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
            Cancel
          </Button>
          <Button onClick={handleStart} disabled={isStarting} className="cursor-pointer">
            <IconRocket className="h-4 w-4 mr-2" />
            {isStarting ? "Starting..." : "Start Chat"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
});

/** Alias for backwards compatibility */
export const QuickChatDialog = QuickChatPickerDialog;
