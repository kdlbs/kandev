import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import TaskLink from "@/components/routing/task-link";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useCopilotStore } from "@/hooks/domains/coordinator/copilot-store";
import { cn } from "@/lib/utils";
import type { AttentionStall, AttentionTask, NeedsYouItem } from "@/lib/coordinator/attention";
import type { CopilotItemRef } from "@/lib/coordinator/copilot-id";
import { Spinner } from "@kandev/ui/spinner";
import { usePhase2CardContext } from "../proposal-card/phase2-context";
import { resumeStalledTask, useStallResume, type StallResumeEntry } from "../use-stall-resume";
import { StallEvidenceContent } from "./stall-evidence-content";

const NOT_RESUMABLE_STATES = new Set(["COMPLETED", "CREATED"]);

/** The stalled task's primary session id when a resume can pick it up, else undefined. */
export function resumableSessionId(task: AttentionTask): string | undefined {
  const session = task.statusSummary?.primary_session;
  if (!session?.id || (session.state && NOT_RESUMABLE_STATES.has(session.state))) return undefined;
  return session.id;
}

type TFn = ReturnType<typeof useTranslation>["t"];

function resumeStatusText(entry: StallResumeEntry | undefined, t: TFn): string {
  if (entry?.phase === "resuming") return t("coordinator:resumeStatusResuming");
  if (entry?.phase === "queued") return t("coordinator:resumeStatusQueued");
  if (entry?.phase === "error") {
    return entry.unknown ? t("coordinator:resumeErrorUnknown") : t("coordinator:resumeError");
  }
  return "";
}

function resumeLabel(entry: StallResumeEntry | undefined, t: TFn): string {
  if (entry?.phase === "resuming") return t("coordinator:resuming");
  if (entry?.phase === "queued") return t("coordinator:resumeQueued");
  return t("coordinator:resume");
}

function ResumeAction({ task, sessionId }: { task: AttentionTask; sessionId: string }) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const entry = useStallResume(task.id);
  const locked = entry !== undefined && entry.phase !== "error";
  const status = resumeStatusText(entry, t);
  return (
    <>
      <Button
        variant="outline"
        size="sm"
        disabled={locked}
        className={cn("cursor-pointer", !isFinePointer && "min-h-11 min-w-11")}
        onClick={() => resumeStalledTask(task.id, sessionId)}
      >
        {entry?.phase === "sending" && <Spinner aria-hidden="true" className="mr-1.5" />}
        {resumeLabel(entry, t)}
      </Button>
      <span role="status" className="sr-only">
        {entry?.phase === "error" ? "" : status}
      </span>
      {entry?.phase === "error" && (
        <p role="alert" className="text-destructive w-full text-xs/relaxed">
          {status}
        </p>
      )}
    </>
  );
}

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
  canManage?: boolean;
};

/**
 * The item's primary actions, by kind: question/error offer Open task only;
 * stall offers Open task and Show the evidence; a proposal offers neither
 * (AC-COORDINATOR-NEEDS-YOU-002.5/.6/.7/.8).
 */
export function NeedsYouItemPrimaryActions({
  item,
  canManage = false,
}: NeedsYouItemPrimaryActionsProps) {
  const { enabled } = usePhase2CardContext();
  if (item.kind === "proposal") return null;
  const sessionId =
    item.kind === "stall" && enabled && canManage ? resumableSessionId(item.task) : undefined;
  return (
    <div className="flex flex-wrap items-center gap-2">
      {sessionId && <ResumeAction task={item.task} sessionId={sessionId} />}
      <OpenTaskAction task={item.task} />
      {item.kind === "stall" && <ShowEvidenceAction task={item.task} stall={item.stall} />}
    </div>
  );
}

export type AskAboutThisButtonProps = {
  coordinatorId: string;
  /** The card's derived `<id>` (`lib/coordinator/copilot-id.ts`). */
  id: string;
  /** The wire reference for `<id>` (`lib/coordinator/copilot-id.ts`). Named
   *  `itemRef`, not `ref`: `ref` is a reserved JSX prop that React would
   *  intercept instead of forwarding it as a normal prop. */
  itemRef: CopilotItemRef;
  canManage: boolean;
};

/**
 * Opens the copilot with a chip and a pre-filled question for this item's
 * `<id>` (docs/specs/coordinator/system-design/copilot-panel.md#ask-about-this).
 * A reader sees the same disabled button with a tooltip and no handler
 * (AC-COORDINATOR-COPILOT-004.8).
 */
export function AskAboutThisButton({
  coordinatorId,
  id,
  itemRef,
  canManage,
}: AskAboutThisButtonProps) {
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
        askAboutThis(coordinatorId, id, itemRef, t("coordinator:copilotQuestionForItem", { id }))
      }
    >
      {t("coordinator:askAboutThis")}
    </Button>
  );
}
