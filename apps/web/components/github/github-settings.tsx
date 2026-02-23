"use client";

import { useState, useCallback } from "react";
import { IconBrandGithub, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { GitHubStatusCard } from "./github-status";
import { ReviewWatchTable } from "./review-watch-table";
import { ReviewWatchDialog } from "./review-watch-dialog";
import { PRStatsPanel } from "./pr-stats";
import { useReviewWatches } from "@/hooks/domains/github/use-review-watches";
import type { ReviewWatch } from "@/lib/types/github";

type GitHubSettingsProps = {
  workspaceId: string;
};

function useWatchActions(workspaceId: string | null) {
  const { items: watches, create, update, remove, trigger } = useReviewWatches(workspaceId);
  const { toast } = useToast();

  const handleDelete = useCallback(
    async (id: string) => {
      try {
        await remove(id);
        toast({ description: "Review watch deleted", variant: "success" });
      } catch {
        toast({ description: "Failed to delete review watch", variant: "error" });
      }
    },
    [remove, toast],
  );

  const handleTrigger = useCallback(
    async (id: string) => {
      try {
        const result = await trigger(id);
        const count = result?.new_prs_found ?? 0;
        if (count > 0) {
          toast({
            description: `Found ${count} new PR${count > 1 ? "s" : ""}`,
            variant: "success",
          });
        } else {
          toast({ description: "No new PRs found" });
        }
      } catch {
        toast({ description: "Failed to check for PRs", variant: "error" });
      }
    },
    [trigger, toast],
  );

  const handleToggleEnabled = useCallback(
    async (watch: ReviewWatch) => {
      try {
        await update(watch.id, { enabled: !watch.enabled });
        toast({
          description: watch.enabled ? "Watch paused" : "Watch enabled",
          variant: "success",
        });
      } catch {
        toast({ description: "Failed to update watch", variant: "error" });
      }
    },
    [update, toast],
  );

  return { watches, create, update, handleDelete, handleTrigger, handleToggleEnabled };
}

export function GitHubSettings({ workspaceId }: GitHubSettingsProps) {
  const { watches, create, update, handleDelete, handleTrigger, handleToggleEnabled } =
    useWatchActions(workspaceId);
  const { toast } = useToast();

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingWatch, setEditingWatch] = useState<ReviewWatch | null>(null);

  const handleEdit = useCallback((watch: ReviewWatch) => {
    setEditingWatch(watch);
    setDialogOpen(true);
  }, []);

  return (
    <TooltipProvider>
      <div className="space-y-8">
        <div>
          <h2 className="text-2xl font-bold flex items-center gap-2">
            <IconBrandGithub className="h-6 w-6" />
            GitHub Integration
          </h2>
          <p className="text-sm text-muted-foreground mt-1">
            Configure GitHub PR monitoring and review queue automation.
          </p>
        </div>

        <Separator />

        <SettingsSection title="Connection Status" description="GitHub authentication status">
          <Card>
            <CardContent className="py-3">
              <GitHubStatusCard />
            </CardContent>
          </Card>
        </SettingsSection>

        <SettingsSection
          title="Review Watches"
          description="Automatically create tasks for PRs that need your review."
          action={
            <Button
              size="sm"
              onClick={() => {
                setEditingWatch(null);
                setDialogOpen(true);
              }}
              className="cursor-pointer"
            >
              <IconPlus className="h-4 w-4 mr-1" />
              Add Watch
            </Button>
          }
        >
          <Card>
            <CardContent className="p-0">
              <ReviewWatchTable
                watches={watches}
                onEdit={handleEdit}
                onDelete={handleDelete}
                onTrigger={handleTrigger}
                onToggleEnabled={handleToggleEnabled}
              />
            </CardContent>
          </Card>
        </SettingsSection>

        <SettingsSection
          title="PR Analytics"
          description="Pull request activity for this workspace."
        >
          <PRStatsPanel workspaceId={workspaceId} />
        </SettingsSection>

        <ReviewWatchDialog
          open={dialogOpen}
          onOpenChange={setDialogOpen}
          watch={editingWatch}
          workspaceId={workspaceId}
          onCreate={async (req) => {
            await create(req);
            toast({ description: "Review watch created", variant: "success" });
          }}
          onUpdate={async (id, req) => {
            await update(id, req);
            toast({ description: "Review watch updated", variant: "success" });
          }}
        />
      </div>
    </TooltipProvider>
  );
}
