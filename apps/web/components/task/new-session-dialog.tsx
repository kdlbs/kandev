"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconGitBranch, IconCopy, IconSparkles } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { addSessionPanel } from "@/lib/state/dockview-panel-actions";
import { replaceSessionUrl } from "@/lib/links";
import { AgentSelector } from "@/components/task-create-dialog-selectors";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";
import type { AgentProfileOption } from "@/lib/state/slices";

type NewSessionDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: string;
};

type ContextMode = "blank" | "copy_prompt";

function useNewSessionDialogState(taskId: string) {
  const taskTitle = useAppStore((state) => {
    const task = state.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return task?.title ?? "Task";
  });
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const currentSession = useAppStore((state) => {
    return activeSessionId ? state.taskSessions.items[activeSessionId] ?? null : null;
  });
  const worktreeBranch = useAppStore((state) => {
    if (!activeSessionId) return null;
    const wtIds = state.sessionWorktreesBySessionId.itemsBySessionId[activeSessionId];
    if (wtIds?.length) {
      const wt = state.worktrees.items[wtIds[0]];
      if (wt?.branch) return wt.branch;
    }
    return currentSession?.worktree_branch ?? null;
  });
  const lastSessionPrompt = useAppStore((state) => {
    if (!activeSessionId) return null;
    const msgs = state.messages.bySession[activeSessionId];
    if (!msgs?.length) return null;
    const first = msgs.find((m: { author_type?: string }) => m.author_type === "user");
    return first ? (first as { content?: string }).content ?? null : null;
  });
  const agentName = useMemo(() => {
    const s = currentSession?.agent_profile_snapshot;
    if (!s) return null;
    return (s.agent_name as string) ?? (s.name as string) ?? null;
  }, [currentSession?.agent_profile_snapshot]);

  return { taskTitle, agentProfiles, currentSession, worktreeBranch, lastSessionPrompt, agentName };
}

function EnvironmentBadges({ agentName, worktreeBranch }: { agentName: string | null; worktreeBranch: string | null }) {
  return (
    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
      {agentName && <Badge variant="secondary" className="text-xs font-normal">{agentName}</Badge>}
      {worktreeBranch && (
        <Badge variant="outline" className="text-xs font-normal gap-1">
          <IconGitBranch className="h-3 w-3" />
          {worktreeBranch}
        </Badge>
      )}
      <span>Same environment as current session</span>
    </div>
  );
}

function ContextChips({ contextMode, onValueChange, hasLastPrompt }: { contextMode: ContextMode; onValueChange: (v: string) => void; hasLastPrompt: boolean }) {
  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium text-muted-foreground">Context</label>
      <ToggleGroup type="single" value={contextMode} onValueChange={onValueChange} className="justify-start">
        <ToggleGroupItem value="blank" className="text-xs cursor-pointer">Blank</ToggleGroupItem>
        <ToggleGroupItem value="copy_prompt" disabled={!hasLastPrompt} className="text-xs gap-1 cursor-pointer">
          <IconCopy className="h-3 w-3" />Copy last prompt
        </ToggleGroupItem>
        <Tooltip>
          <TooltipTrigger asChild>
            <span><ToggleGroupItem value="summarize" disabled className="text-xs gap-1"><IconSparkles className="h-3 w-3" />Summarize</ToggleGroupItem></span>
          </TooltipTrigger>
          <TooltipContent>Coming soon</TooltipContent>
        </Tooltip>
      </ToggleGroup>
    </div>
  );
}

function NewSessionForm({ taskId, defaultProfileId, executorId, agentName, worktreeBranch, lastSessionPrompt, agentProfiles, onClose }: {
  taskId: string; defaultProfileId: string; executorId: string; agentName: string | null;
  worktreeBranch: string | null; lastSessionPrompt: string | null; agentProfiles: AgentProfileOption[]; onClose: () => void;
}) {
  const { toast } = useToast();
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const [isCreating, setIsCreating] = useState(false);
  const [contextMode, setContextMode] = useState<ContextMode>("blank");
  const [selectedProfileId, setSelectedProfileId] = useState(defaultProfileId);
  const promptRef = useRef<HTMLTextAreaElement>(null);
  const profileOptions = useAgentProfileOptions(agentProfiles);

  const handleContextModeChange = useCallback((value: string) => {
    if (!value) return;
    setContextMode(value as ContextMode);
    if (value === "copy_prompt" && lastSessionPrompt && promptRef.current) promptRef.current.value = lastSessionPrompt;
    else if (value === "blank" && promptRef.current) promptRef.current.value = "";
  }, [lastSessionPrompt]);

  const handleSubmit = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    const typed = promptRef.current?.value?.trim() ?? "";
    const prompt = contextMode === "copy_prompt" && !typed && lastSessionPrompt ? lastSessionPrompt : typed;
    if (!prompt) return;
    setIsCreating(true);
    try {
      const { request } = buildStartRequest(taskId, selectedProfileId || defaultProfileId, { executorId, prompt });
      const response = await launchSession(request);
      if (response.session_id) {
        // Pre-set currentLayoutSessionId to prevent switchSessionLayout from
        // doing a full layout teardown/rebuild. We want to keep the existing
        // layout (terminal, files, changes) and just add a new session tab.
        useDockviewStore.setState({ currentLayoutSessionId: response.session_id });
        setActiveSession(taskId, response.session_id);
        const { api, centerGroupId } = useDockviewStore.getState();
        if (api) addSessionPanel(api, centerGroupId, response.session_id, "Agent");
        replaceSessionUrl(response.session_id);
      }
      onClose();
    } catch (error) {
      toast({ title: "Failed to create session", description: error instanceof Error ? error.message : "Unknown error", variant: "error" });
    } finally {
      setIsCreating(false);
    }
  }, [taskId, selectedProfileId, defaultProfileId, executorId, contextMode, lastSessionPrompt, onClose, toast, setActiveSession]);

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <EnvironmentBadges agentName={agentName} worktreeBranch={worktreeBranch} />
      {profileOptions.length > 1 && (
        <div className="space-y-1.5">
          <label className="text-xs font-medium text-muted-foreground">Agent</label>
          <AgentSelector options={profileOptions} value={selectedProfileId || defaultProfileId} onValueChange={setSelectedProfileId} disabled={isCreating} placeholder="Select agent profile" />
        </div>
      )}
      <ContextChips contextMode={contextMode} onValueChange={handleContextModeChange} hasLastPrompt={!!lastSessionPrompt} />
      <textarea
        ref={promptRef}
        placeholder="What should the agent work on?"
        className="w-full min-h-[100px] rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring resize-none"
        autoFocus
        disabled={isCreating}
        onKeyDown={(e) => { if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); handleSubmit(e); } }}
      />
      <DialogFooter>
        <Button type="button" variant="ghost" onClick={onClose} disabled={isCreating} className="cursor-pointer">Cancel</Button>
        <Button type="submit" disabled={isCreating} className="cursor-pointer">{isCreating ? "Creating..." : "Start Session"}</Button>
      </DialogFooter>
    </form>
  );
}

export function NewSessionDialog({ open, onOpenChange, taskId }: NewSessionDialogProps) {
  const { taskTitle, agentProfiles, currentSession, worktreeBranch, lastSessionPrompt, agentName } =
    useNewSessionDialogState(taskId);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle className="text-sm font-medium">
            New session in <span className="text-foreground">{taskTitle}</span>
          </DialogTitle>
        </DialogHeader>
        <NewSessionForm
          key={`${open}`}
          taskId={taskId}
          defaultProfileId={currentSession?.agent_profile_id ?? ""}
          executorId={currentSession?.executor_id ?? ""}
          agentName={agentName}
          worktreeBranch={worktreeBranch}
          lastSessionPrompt={lastSessionPrompt}
          agentProfiles={agentProfiles}
          onClose={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}
