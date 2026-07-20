import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

export function WorkflowSyncedBadge({ sourcePath }: { sourcePath?: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge
          variant="outline"
          tabIndex={0}
          className="text-xs cursor-default"
          data-testid="workflow-synced-badge"
        >
          Synced
        </Badge>
      </TooltipTrigger>
      <TooltipContent>
        Read-only - managed by workflow sync from {sourcePath || "a configured repository"}. Edit or
        remove it in the synced repository.
      </TooltipContent>
    </Tooltip>
  );
}
