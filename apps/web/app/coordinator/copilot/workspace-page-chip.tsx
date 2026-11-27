import { useTranslation } from "react-i18next";
import { IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { CopilotChip } from "@/hooks/domains/coordinator/copilot-store";
import type { CoordinatorWatchProjects } from "@/lib/api/domains/coordinator-api";
import { ProjectsHint } from "./projects-hint";

type WorkspacePageChipRowProps = {
  chip: CopilotChip;
  /** The coordinator does not watch the page's workflow. */
  notWatched: boolean;
  projects?: CoordinatorWatchProjects;
  onRemove: () => void;
};

/** The page chip above the composer: "This task: `<identifier>`" or
 *  "This board: `<name>`", with the watched hint beside it. Only the id leaves
 *  the page; the tooltip says so. */
export function WorkspacePageChipRow({
  chip,
  notWatched,
  projects,
  onRemove,
}: WorkspacePageChipRowProps) {
  const { t } = useTranslation();
  const text =
    chip.ref.kind === "workflow"
      ? t("coordinator:copilotPageChipBoard", { label: chip.label })
      : t("coordinator:copilotPageChipTask", { label: chip.label });
  return (
    <div
      className="flex shrink-0 flex-wrap items-center gap-2 border-b px-3 py-1.5"
      data-testid="workspace-copilot-chip-row"
    >
      <Badge variant="secondary" className="w-fit gap-1 pr-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <span tabIndex={0} data-testid="workspace-copilot-chip-label">
              {text}
            </span>
          </TooltipTrigger>
          <TooltipContent>{t("coordinator:copilotPageChipTooltip")}</TooltipContent>
        </Tooltip>
        <Button
          size="icon-sm"
          variant="ghost"
          className="cursor-pointer [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
          onClick={onRemove}
          aria-label={t("coordinator:copilotRemoveChip")}
        >
          <IconX className="h-3 w-3" />
        </Button>
      </Badge>
      {notWatched && (
        <span className="text-xs text-muted-foreground" data-testid="workspace-copilot-not-watched">
          {t("coordinator:copilotNotWatched")}
        </span>
      )}
      <ProjectsHint projects={projects} />
    </div>
  );
}
