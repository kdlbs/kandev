import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import type { QueueGroupKind, QueueItem } from "@/lib/coordinator/attention";
import { QueueRow } from "./queue-row";

const GROUP_LABEL_KEY: Record<QueueGroupKind, string> = {
  working: "coordinator:groupWorking",
  in_review: "coordinator:groupInReview",
  ready_to_merge: "coordinator:groupReadyToMerge",
  done: "coordinator:groupDone",
  other: "coordinator:groupOther",
};

const COLLAPSED_BY_DEFAULT: ReadonlySet<QueueGroupKind> = new Set(["done", "other"]);

export type QueueGroupProps = {
  group: QueueGroupKind;
  items: QueueItem[];
  stepNameByTaskId: Map<string, string>;
  defaultOpen?: boolean;
};

function QueueGroupRows({
  items,
  stepNameByTaskId,
}: {
  items: QueueItem[];
  stepNameByTaskId: Map<string, string>;
}) {
  return (
    <div className="divide-border divide-y" data-testid="queue-group-rows">
      {items.map((item) => (
        <QueueRow key={item.id} item={item} stepNameByTaskId={stepNameByTaskId} />
      ))}
    </div>
  );
}

/**
 * One Queue group: Working/In review/Ready to merge shown expanded, Done and
 * Other collapsed behind a disclosure (AC-COORDINATOR-NEEDS-YOU-004.1).
 */
export function QueueGroup({ group, items, stepNameByTaskId, defaultOpen }: QueueGroupProps) {
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
        <h3 className="flex items-center gap-2 text-sm font-medium">
          <span>{label}</span>
          <Badge variant="secondary">{items.length}</Badge>
        </h3>
        <QueueGroupRows items={items} stepNameByTaskId={stepNameByTaskId} />
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
          <span className="text-sm font-medium">{label}</span>
          <Badge variant="secondary">{items.length}</Badge>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <QueueGroupRows items={items} stepNameByTaskId={stepNameByTaskId} />
      </CollapsibleContent>
    </Collapsible>
  );
}
