"use client";

import { useCallback, useMemo, useState } from "react";
import { IconCopy } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import {
  copyIntegrationConfig,
  integrationLabel,
  type IntegrationSlug,
} from "./integration-copy-config";

type Workspace = { id: string; name: string };

type IntegrationCopyConfigMenuProps = {
  slug: IntegrationSlug;
  sourceWorkspaceId: string;
  workspaces: Workspace[];
};

function workspaceName(workspaces: Workspace[], id: string | null): string {
  return workspaces.find((w) => w.id === id)?.name ?? "";
}

function CopyConfigDialogBody({
  label,
  slug,
  sourceName,
  targets,
  targetId,
  setTargetId,
  copying,
  onCopy,
  onCancel,
}: {
  label: string;
  slug: IntegrationSlug;
  sourceName: string;
  targets: Workspace[];
  targetId: string | null;
  setTargetId: (id: string) => void;
  copying: boolean;
  onCopy: () => void;
  onCancel: () => void;
}) {
  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Copy {label} configuration</DialogTitle>
        <DialogDescription>
          Copy the {label} settings
          {slug === "github" ? "" : " and credentials"} from{" "}
          <span className="font-medium text-foreground">{sourceName}</span> into another workspace.
          This overwrites the target workspace&apos;s current {label} configuration. Watchers and
          automations are not copied.
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-2">
        <label htmlFor="copy-config-target" className="text-xs font-medium text-muted-foreground">
          Target workspace
        </label>
        <Select value={targetId ?? undefined} onValueChange={setTargetId}>
          <SelectTrigger
            id="copy-config-target"
            className="w-full"
            data-testid="integration-copy-config-target"
          >
            <SelectValue placeholder="Select a workspace…" />
          </SelectTrigger>
          <SelectContent>
            {targets.map((workspace) => (
              <SelectItem key={workspace.id} value={workspace.id}>
                {workspace.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" className="cursor-pointer" onClick={onCancel}>
          Cancel
        </Button>
        <Button
          type="button"
          className="cursor-pointer"
          disabled={!targetId || copying}
          onClick={onCopy}
          data-testid="integration-copy-config-confirm"
        >
          <IconCopy className="h-4 w-4" />
          Copy config
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}

// IntegrationCopyConfigMenu renders an icon-only button next to the workspace
// switcher. Clicking it opens a dialog that explains what copying does and lets
// the user pick a target workspace to copy the current integration's config
// (and credentials, except GitHub) into.
export function IntegrationCopyConfigMenu({
  slug,
  sourceWorkspaceId,
  workspaces,
}: IntegrationCopyConfigMenuProps) {
  const { toast, updateToast } = useToast();
  const [open, setOpen] = useState(false);
  const [copying, setCopying] = useState(false);
  const [targetId, setTargetId] = useState<string | null>(null);
  const label = integrationLabel(slug);

  const targets = useMemo(
    () => workspaces.filter((w) => w.id !== sourceWorkspaceId),
    [workspaces, sourceWorkspaceId],
  );
  const sourceName = workspaceName(workspaces, sourceWorkspaceId);

  const onCopy = useCallback(async () => {
    if (!targetId) return;
    const targetName = workspaceName(workspaces, targetId);
    setCopying(true);
    const pendingId = toast({
      variant: "loading",
      description: `Copying ${label} config to ${targetName}…`,
    });
    try {
      await copyIntegrationConfig(slug, sourceWorkspaceId, targetId);
      updateToast(pendingId, {
        variant: "success",
        description: `Copied ${label} config to ${targetName}.`,
      });
      setOpen(false);
      setTargetId(null);
    } catch (err) {
      updateToast(pendingId, {
        variant: "error",
        description: `Failed to copy config: ${String(err)}`,
      });
    } finally {
      setCopying(false);
    }
  }, [slug, sourceWorkspaceId, targetId, workspaces, label, toast, updateToast]);

  if (targets.length === 0) return null;

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="icon-lg"
            className="cursor-pointer"
            aria-label="Copy config to another workspace"
            data-testid="integration-copy-config-trigger"
            onClick={() => setOpen(true)}
          >
            <IconCopy className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Copy config to another workspace</TooltipContent>
      </Tooltip>
      <CopyConfigDialogBody
        label={label}
        slug={slug}
        sourceName={sourceName}
        targets={targets}
        targetId={targetId}
        setTargetId={setTargetId}
        copying={copying}
        onCopy={() => void onCopy()}
        onCancel={() => setOpen(false)}
      />
    </Dialog>
  );
}
