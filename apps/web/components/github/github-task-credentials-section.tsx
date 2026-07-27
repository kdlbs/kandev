"use client";

import { useCallback, useEffect, useState } from "react";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsSection } from "@/components/settings/settings-section";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useToast } from "@/components/toast-provider";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import {
  fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings,
} from "@/lib/api/domains/github-api";
import type { TaskGitCredentialsMode } from "@/lib/types/github";

const deliveryHelp =
  "PAT, named GitHub CLI, and GitHub App connections choose Kandev's workspace automation identity. In managed mode Kandev brokers that identity to the task for GitHub HTTPS and gh. Inherit executor credentials leaves task Git and gh to the selected executor. An explicit executor-profile GH_TOKEN or GITHUB_TOKEN takes precedence in managed mode.";

function TaskCredentialsHelp() {
  const usesDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const button = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="h-11 w-11 cursor-pointer text-muted-foreground sm:h-7 sm:w-7"
      aria-label="Explain task Git credentials"
    >
      <IconInfoCircle className="h-4 w-4" />
    </Button>
  );
  const trigger = <DrawerTrigger asChild>{button}</DrawerTrigger>;
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      {usesDrawer ? (
        trigger
      ) : (
        <Tooltip>
          <TooltipTrigger asChild>{trigger}</TooltipTrigger>
          <TooltipContent className="max-w-[320px] text-xs leading-relaxed">
            {deliveryHelp}
          </TooltipContent>
        </Tooltip>
      )}
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>How task Git credentials work</DrawerTitle>
          <DrawerDescription>{deliveryHelp}</DrawerDescription>
        </DrawerHeader>
      </DrawerContent>
    </Drawer>
  );
}

export function GitHubTaskCredentialsSection({ workspaceId }: { workspaceId: string }) {
  const { toast } = useToast();
  const [mode, setMode] = useState<TaskGitCredentialsMode>("managed");
  const [baseline, setBaseline] = useState<TaskGitCredentialsMode>("managed");
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    void fetchGitHubWorkspaceSettings(workspaceId)
      .then((settings) => {
        if (!cancelled) {
          const next = settings.task_git_credentials_mode ?? "managed";
          setMode(next);
          setBaseline(next);
        }
      })
      .catch(() => {
        if (!cancelled)
          toast({ description: "Failed to load task Git credential settings", variant: "error" });
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [toast, workspaceId]);
  const save = useCallback(async () => {
    try {
      const updated = await updateGitHubWorkspaceSettings({
        workspace_id: workspaceId,
        task_git_credentials_mode: mode,
      });
      const next = updated.task_git_credentials_mode ?? "managed";
      setBaseline(next);
      setMode((current) => (current === mode ? next : current));
      toast({ description: "Task Git credential settings saved", variant: "success" });
    } catch {
      toast({ description: "Failed to save task Git credential settings", variant: "error" });
      throw new Error("Failed to save task Git credential settings");
    }
  }, [mode, toast, workspaceId]);
  const dirty = mode !== baseline;
  useSettingsSaveContributor({
    id: `github-task-credentials:${workspaceId}`,
    revision: mode,
    isDirty: dirty,
    canSave: !loading,
    save,
    discard: () => setMode(baseline),
  });
  return (
    <SettingsSection
      title="Task Git credentials"
      description="Choose how task processes authenticate to GitHub. This does not change Kandev's workspace automation identity."
      action={<TaskCredentialsHelp />}
    >
      <SettingsCard isDirty={dirty}>
        <CardContent className="space-y-4 py-4">
          <RadioGroup
            value={mode}
            onValueChange={(value) => setMode(value as TaskGitCredentialsMode)}
            disabled={loading}
            data-testid="github-task-credentials-mode"
          >
            <label className="flex cursor-pointer items-start gap-3 rounded-md border p-3">
              <RadioGroupItem value="managed" className="mt-0.5" />
              <span>
                <span className="font-medium">Managed workspace credentials</span>
                <span className="mt-1 block text-sm text-muted-foreground">
                  Kandev brokers the workspace PAT, named GitHub CLI account, or App identity to
                  this task for GitHub HTTPS and gh.
                </span>
              </span>
            </label>
            <label className="flex cursor-pointer items-start gap-3 rounded-md border p-3">
              <RadioGroupItem value="executor" className="mt-0.5" />
              <span>
                <span className="font-medium">Inherit executor Git credentials</span>
                <span className="mt-1 block text-sm text-muted-foreground">
                  Local and Worktree tasks use host-visible Git or SSH credentials. Docker, SSH, and
                  cloud tasks use credentials configured in that executor.
                </span>
              </span>
            </label>
          </RadioGroup>
          <p className="text-xs text-muted-foreground">
            An executor-profile GH_TOKEN or GITHUB_TOKEN overrides managed workspace credentials for
            that task.
          </p>
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}
