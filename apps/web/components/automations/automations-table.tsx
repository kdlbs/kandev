"use client";

import { useRouter } from "@/lib/routing/client-router";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconPlayerPlay, IconTrash } from "@tabler/icons-react";
import type { Automation, TriggerType } from "@/lib/types/automation";
import { formatRelativeTime } from "./format-utils";

type AutomationsTableProps = {
  automations: Automation[];
  workspaceId: string;
  onToggleEnabled: (id: string, enabled: boolean) => void;
  onTrigger: (id: string) => void;
  onDelete: (id: string) => void;
};

const TRIGGER_BADGE_VARIANT: Record<TriggerType, string> = {
  scheduled: "bg-blue-500/15 text-blue-400 border-blue-500/20",
  github_pr: "bg-purple-500/15 text-purple-400 border-purple-500/20",
  github_push: "bg-purple-500/15 text-purple-400 border-purple-500/20",
  github_ci: "bg-purple-500/15 text-purple-400 border-purple-500/20",
  webhook: "bg-orange-500/15 text-orange-400 border-orange-500/20",
};

const TRIGGER_LABELS: Record<TriggerType, string> = {
  scheduled: "Scheduled",
  github_pr: "GitHub PR",
  github_push: "GitHub Push",
  github_ci: "GitHub CI",
  webhook: "Webhook",
};

function TriggerBadges({ triggers }: { triggers: Automation["triggers"] }) {
  const items = triggers ?? [];
  if (items.length === 0) {
    return <span className="text-xs text-muted-foreground">None</span>;
  }
  return (
    <div className="flex flex-wrap gap-1">
      {items.map((t) => (
        <Badge key={t.id} variant="outline" className={TRIGGER_BADGE_VARIANT[t.type]}>
          {TRIGGER_LABELS[t.type]}
        </Badge>
      ))}
    </div>
  );
}

function RowActions({
  id,
  onTrigger,
  onDelete,
}: {
  id: string;
  onTrigger: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  return (
    <div className="flex items-center justify-end gap-0.5">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            className="cursor-pointer"
            onClick={() => onTrigger(id)}
          >
            <IconPlayerPlay className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Trigger manually</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            className="cursor-pointer"
            onClick={() => onDelete(id)}
          >
            <IconTrash className="h-3.5 w-3.5 text-destructive" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Delete</TooltipContent>
      </Tooltip>
    </div>
  );
}

export function AutomationsTable({
  automations,
  workspaceId,
  onToggleEnabled,
  onTrigger,
  onDelete,
}: AutomationsTableProps) {
  const router = useRouter();

  return (
    <div className="rounded-md border" data-testid="automations-table">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent focus-within:bg-transparent">
            <TableHead>Name</TableHead>
            <TableHead>Mode</TableHead>
            <TableHead>Triggers</TableHead>
            <TableHead>Enabled</TableHead>
            <TableHead>Last Triggered</TableHead>
            <TableHead className="w-[100px]" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {automations.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={6}
                className="text-center text-muted-foreground py-8"
                data-testid="automations-empty"
              >
                No automations yet
              </TableCell>
            </TableRow>
          ) : (
            automations.map((a) => (
              <TableRow
                key={a.id}
                data-testid={`automation-row-${a.id}`}
                className="cursor-pointer hover:bg-muted/50"
                onClick={() =>
                  router.push(`/settings/workspace/${workspaceId}/automations/${a.id}`)
                }
              >
                <TableCell className="font-medium">{a.name}</TableCell>
                <TableCell>
                  <Badge variant="outline" className="text-[10px]">
                    {a.execution_mode === "run" ? "Run" : "Task"}
                  </Badge>
                </TableCell>
                <TableCell>
                  <TriggerBadges triggers={a.triggers} />
                </TableCell>
                <TableCell onClick={(e) => e.stopPropagation()}>
                  <Switch
                    data-testid={`automation-enabled-${a.id}`}
                    size="sm"
                    checked={a.enabled}
                    onCheckedChange={(checked) => onToggleEnabled(a.id, checked)}
                    className="cursor-pointer"
                  />
                </TableCell>
                <TableCell className="text-muted-foreground text-sm">
                  {a.last_triggered_at ? formatRelativeTime(a.last_triggered_at) : "Never"}
                </TableCell>
                <TableCell onClick={(e) => e.stopPropagation()}>
                  <RowActions id={a.id} onTrigger={onTrigger} onDelete={onDelete} />
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  );
}
