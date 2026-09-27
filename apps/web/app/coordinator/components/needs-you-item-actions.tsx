import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverTrigger } from "@kandev/ui/popover";
import TaskLink from "@/components/routing/task-link";
import type { AttentionStall, AttentionTask, NeedsYouItem } from "@/lib/coordinator/attention";
import { StallEvidenceContent } from "./stall-evidence-content";

function OpenTaskAction({ task }: { task: AttentionTask }) {
  const { t } = useTranslation();
  return (
    <Button asChild variant="outline" size="sm">
      <TaskLink taskId={task.id}>{t("coordinator:openTask")}</TaskLink>
    </Button>
  );
}

function ShowEvidenceAction({ task, stall }: { task: AttentionTask; stall: AttentionStall }) {
  const { t } = useTranslation();
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm">
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

/**
 * Renders on every item as a native disabled button with no click handler
 * (out of scope here; task 06 enables and wires it).
 */
export function AskAboutThisButton() {
  const { t } = useTranslation();
  return (
    <Button variant="ghost" size="sm" disabled>
      {t("coordinator:askAboutThis")}
    </Button>
  );
}
