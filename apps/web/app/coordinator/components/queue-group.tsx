import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { cn } from "@/lib/utils";
import type { QueueGroupKind, QueueItem } from "@/lib/coordinator/attention";
import type { TaskPR } from "@/lib/types/github";
import { QueueRow } from "./queue-row";

const GROUP_LABEL_KEY: Record<QueueGroupKind, string> = {
  working: "coordinator:groupWorking",
  in_review: "coordinator:groupInReview",
  ready_to_merge: "coordinator:groupReadyToMerge",
  done: "coordinator:groupDone",
  other: "coordinator:groupOther",
};

const COLLAPSED_BY_DEFAULT: ReadonlySet<QueueGroupKind> = new Set(["done", "other"]);

/**
 * The group heading: a small uppercase eyebrow, not a title competing with
 * the item cards below it (mockup v2.1 `h2`).
 */
const GROUP_HEADING_CLASS =
  "text-muted-foreground text-[10px] font-semibold tracking-[0.09em] uppercase";

export type QueueGroupProps = {
  group: QueueGroupKind;
  items: QueueItem[];
  stepNameByTaskId: Map<string, string>;
  prsByTaskId: ReadonlyMap<string, TaskPR[]>;
  defaultOpen?: boolean;
  /** Phase 2 on: Ready to merge rows get their actions and the group its header line. */
  phase2?: boolean;
  canManage?: boolean;
};

function QueueGroupRows({
  items,
  stepNameByTaskId,
  prsByTaskId,
  phase2,
  canManage,
}: {
  items: QueueItem[];
  stepNameByTaskId: Map<string, string>;
  prsByTaskId: ReadonlyMap<string, TaskPR[]>;
  phase2?: boolean;
  canManage?: boolean;
}) {
  return (
    // Rows sit in one bordered panel rather than floating on the page
    // background (mockup v2.1 `.card` around `.row`).
    <div
      className="border-border bg-card divide-border divide-y rounded-md border"
      data-testid="queue-group-rows"
    >
      {items.map((item) => (
        <QueueRow
          key={item.id}
          item={item}
          stepNameByTaskId={stepNameByTaskId}
          prsByTaskId={prsByTaskId}
          phase2={phase2}
          canManage={canManage}
        />
      ))}
    </div>
  );
}

/**
 * One Queue group: Working/In review/Ready to merge shown expanded, Done and
 * Other collapsed behind a disclosure (AC-COORDINATOR-NEEDS-YOU-004.1).
 */
export function QueueGroup({
  group,
  items,
  stepNameByTaskId,
  prsByTaskId,
  defaultOpen,
  phase2,
  canManage,
}: QueueGroupProps) {
  const { t } = useTranslation();
  const label = t(GROUP_LABEL_KEY[group]);
  const open = defaultOpen ?? !COLLAPSED_BY_DEFAULT.has(group);

  if (!COLLAPSED_BY_DEFAULT.has(group)) {
    return (
      <section
        id={`queue-group-${group}`}
        data-testid={`queue-group-${group}`}
        className="space-y-2"
      >
        <h3 className={cn("flex items-center gap-2", GROUP_HEADING_CLASS)}>
          <span>{label}</span>
          <Badge variant="secondary">{items.length}</Badge>
        </h3>
        {phase2 && group === "ready_to_merge" && (
          <p className="text-muted-foreground text-xs">{t("coordinator:mergeAlwaysHuman")}</p>
        )}
        <QueueGroupRows
          items={items}
          stepNameByTaskId={stepNameByTaskId}
          prsByTaskId={prsByTaskId}
          phase2={phase2}
          canManage={canManage}
        />
      </section>
    );
  }

  return (
    <Collapsible
      defaultOpen={open}
      id={`queue-group-${group}`}
      data-testid={`queue-group-${group}`}
    >
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="flex w-full items-center justify-start gap-2 px-0">
          <span className={GROUP_HEADING_CLASS}>{label}</span>
          <Badge variant="secondary">{items.length}</Badge>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <QueueGroupRows
          items={items}
          stepNameByTaskId={stepNameByTaskId}
          prsByTaskId={prsByTaskId}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}
