import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import TaskLink from "@/components/routing/task-link";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useCopilotStore } from "@/hooks/domains/coordinator/copilot-store";
import { cn } from "@/lib/utils";
import type { AttentionStall, AttentionTask, NeedsYouItem } from "@/lib/coordinator/attention";
import { StallEvidenceContent } from "./stall-evidence-content";

function OpenTaskAction({ task }: { task: AttentionTask }) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  return (
    <Button
      asChild
      variant="outline"
      size="sm"
      className={cn(!isFinePointer && "min-h-11 min-w-11")}
    >
      <TaskLink taskId={task.id}>{t("coordinator:openTask")}</TaskLink>
    </Button>
  );
}

function ShowEvidenceAction({ task, stall }: { task: AttentionTask; stall: AttentionStall }) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className={cn(!isFinePointer && "min-h-11 min-w-11")}>
          {t("coordinator:showEvidence")}
        </Button>
      </PopoverTrigger>
      <StallEvidenceContent task={task} stall={stall} />
    </Popover>
  );
}

export type NeedsYouItemPrimaryActionsProps = {
  item: NeedsYouItem;
};

/**
 * The item's primary actions, by kind: question/error offer Open task only;
 * stall offers Open task and Show the evidence; a proposal offers neither
 * (AC-COORDINATOR-NEEDS-YOU-002.5/.6/.7/.8).
 */
export function NeedsYouItemPrimaryActions({ item }: NeedsYouItemPrimaryActionsProps) {
  if (item.kind === "proposal") return null;
  return (
    <div className="flex flex-wrap items-center gap-2">
      <OpenTaskAction task={item.task} />
      {item.kind === "stall" && <ShowEvidenceAction task={item.task} stall={item.stall} />}
    </div>
  );
}

export type AskAboutThisButtonProps = {
  coordinatorId: string;
  /** The card's derived `<id>` (`lib/coordinator/copilot-id.ts`). */
  id: string;
  canManage: boolean;
};

/**
 * Opens the copilot with a chip and a pre-filled question for this item's
 * `<id>` (docs/specs/coordinator/system-design/copilot-popover.md#ask-about-this).
 * A reader sees the same disabled button with a tooltip and no handler
 * (AC-COORDINATOR-COPILOT-004.8).
 */
export function AskAboutThisButton({ coordinatorId, id, canManage }: AskAboutThisButtonProps) {
  const { t } = useTranslation();
  const askAboutThis = useCopilotStore((s) => s.askAboutThis);

  if (!canManage) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span tabIndex={0} className="inline-flex">
            <Button variant="ghost" size="sm" disabled>
              {t("coordinator:askAboutThis")}
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent>{t("coordinator:copilotReaderTooltip")}</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <Button
      variant="ghost"
      size="sm"
      className="cursor-pointer"
      onClick={() =>
        askAboutThis(coordinatorId, id, t("coordinator:copilotQuestionForItem", { id }))
      }
    >
      {t("coordinator:askAboutThis")}
    </Button>
  );
}
