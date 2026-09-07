"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { IconChevronRight, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { cn } from "@kandev/ui/lib/utils";
import Link from "@/components/routing/app-link";
import { mapWorkspaceItem } from "@/lib/routing/route-bootstrap";
import { createWorkspaceAction } from "@/app/actions/workspaces";
import { useRequest } from "@/lib/http/use-request";
import { useToast } from "@/components/toast-provider";
import { RequestIndicator } from "@/components/request-indicator";
import { useAppStore } from "@/components/state-provider";
import {
  useWorkspaceSectionCounts,
  WorkspaceSectionStats,
} from "@/components/settings/workspaces/workspace-section-links";
import {
  ActiveWorkspaceBadge,
  workspaceSettingsHref,
} from "@/components/settings/workspaces/workspace-settings-shell";
import { orderWorkspacesForDisplay } from "@/lib/settings/workspace-display-order";
import type { WorkspaceState } from "@/lib/state/slices";

type Workspace = WorkspaceState["items"][number];

type AddWorkspaceFormProps = {
  newWorkspaceName: string;
  onNameChange: (value: string) => void;
  onSubmit: (e: React.FormEvent) => void;
  onCancel: () => void;
  isLoading: boolean;
  status: "idle" | "loading" | "success" | "error";
};

function AddWorkspaceForm({
  newWorkspaceName,
  onNameChange,
  onSubmit,
  onCancel,
  isLoading,
  status,
}: AddWorkspaceFormProps) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardContent className="pt-6">
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="workspace-name">{t("workspaces:workspaceName")}</Label>
            <Input
              id="workspace-name"
              value={newWorkspaceName}
              onChange={(e) => onNameChange(e.target.value)}
              placeholder={t("workspaces:workspaceNamePlaceholder")}
              required
              autoFocus
            />
          </div>
          <div className="flex gap-2 justify-end">
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("common:cancel")}
            </Button>
            <div className="flex items-center gap-2">
              <RequestIndicator status={status} />
              <Button type="submit" disabled={isLoading}>
                {t("workspaces:addWorkspace")}
              </Button>
            </div>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

function WorkspaceListItem({ workspace }: { workspace: Workspace }) {
  const { t } = useTranslation();
  const activeId = useAppStore((s) => s.workspaces.activeId);
  const isActive = workspace.id === activeId;
  const { counts, settled } = useWorkspaceSectionCounts(workspace.id);
  const total = Object.values(counts).reduce((sum, value) => sum + (value ?? 0), 0);
  return (
    <Card
      className={cn(
        "relative transition-colors hover:bg-muted/50",
        isActive && "border-l-2 border-l-primary",
      )}
      data-testid="workspace-list-item"
    >
      {/* Whole-card link as an overlay — the section tiles sit above it (z-10). */}
      <Link
        href={workspaceSettingsHref(workspace.id, "overview")}
        aria-label={workspace.name}
        className="absolute inset-0"
        data-testid="workspace-overview-link"
      />
      <CardContent className="flex flex-col gap-3 lg:flex-row lg:items-center lg:gap-6">
        <div className="flex items-center justify-between gap-3 lg:w-44 lg:shrink-0">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h4 className="truncate text-lg font-semibold">{workspace.name}</h4>
              {isActive && <ActiveWorkspaceBadge />}
            </div>
            <p className="mt-0.5 hidden text-sm text-muted-foreground lg:block">
              {settled ? t("workspaces:resourceCount", { count: total }) : "\u00a0"}
            </p>
          </div>
          <IconChevronRight className="h-5 w-5 shrink-0 text-muted-foreground lg:hidden" />
        </div>
        <WorkspaceSectionStats workspaceId={workspace.id} counts={counts} />
        <IconChevronRight className="hidden h-5 w-5 shrink-0 text-muted-foreground lg:block" />
      </CardContent>
    </Card>
  );
}

export function WorkspacesPageClient() {
  const items = useAppStore((state) => state.workspaces.items);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const setWorkspaces = useAppStore((state) => state.setWorkspaces);
  const [isAdding, setIsAdding] = useState(false);
  const [newWorkspaceName, setNewWorkspaceName] = useState("");
  const createRequest = useRequest(createWorkspaceAction);
  const { toast } = useToast();
  const { t } = useTranslation();
  const orderedItems = orderWorkspacesForDisplay(items, activeWorkspaceId);

  const handleAddWorkspace = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newWorkspaceName.trim()) return;
    try {
      const created = await createRequest.run({ name: newWorkspaceName.trim() });
      setWorkspaces([
        // Both lists go through the shared mapper. Hand-copying fields here is
        // how visibility and the caller's scopes got dropped from the store the
        // first time, which silently rendered a workspace read-only to its own
        // owner.
        mapWorkspaceItem(created),
        // Existing entries are already store items; re-listing their fields by
        // hand is what dropped visibility and scopes before.
        ...items,
      ]);
      setNewWorkspaceName("");
      setIsAdding(false);
    } catch (error) {
      toast({
        title: t("workspaces:failedToCreateWorkspace"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
    }
  };

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-2xl font-bold">{t("common:workspaces")}</h2>
          <p className="text-sm text-muted-foreground mt-1">
            {t("workspaces:manageYourWorkspacesAndWorkflows")}
          </p>
        </div>
        <Button size="sm" className={settingsActionClassName()} onClick={() => setIsAdding(true)}>
          <IconPlus className="h-4 w-4 mr-2" />
          {t("workspaces:addWorkspace")}
        </Button>
      </div>

      <Separator />

      <Separator />

      <div className="space-y-4">
        <div className="grid gap-3">
          {isAdding && (
            <AddWorkspaceForm
              newWorkspaceName={newWorkspaceName}
              onNameChange={setNewWorkspaceName}
              onSubmit={handleAddWorkspace}
              onCancel={() => setIsAdding(false)}
              isLoading={createRequest.isLoading}
              status={createRequest.status}
            />
          )}

          {orderedItems.map((workspace: Workspace) => (
            <WorkspaceListItem key={workspace.id} workspace={workspace} />
          ))}

          {orderedItems.length === 0 && (
            <Card>
              <CardContent className="py-8 text-center">
                <p className="text-sm text-muted-foreground">
                  {t("workspaces:noWorkspacesConfigured")}
                </p>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
