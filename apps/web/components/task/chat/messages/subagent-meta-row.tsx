import { Badge } from "@kandev/ui/badge";
import type { SubagentTaskPayload } from "@/components/task/chat/types";
import { subagentMetaChips } from "@/components/task/chat/messages/subagent-meta";

/**
 * Compact row of metadata chips (duration, tokens, tools, model, session) rendered
 * under a completed subagent's header. Renders nothing when no metric fields
 * are present, so the card degrades gracefully across agents.
 */
export function SubagentMetaRow({ subagentTask }: { subagentTask?: SubagentTaskPayload }) {
  const chips = subagentMetaChips(subagentTask);
  if (chips.length === 0) return null;
  return (
    <div
      data-testid="subagent-meta"
      className="flex flex-wrap items-center gap-1.5 mt-1 ml-7 text-muted-foreground"
    >
      {chips.map((chip) => (
        <Badge
          key={chip.label}
          variant="secondary"
          data-testid={`subagent-meta-${chip.label}`}
          className="font-mono text-[10px] font-normal"
        >
          {chip.value}
        </Badge>
      ))}
    </div>
  );
}
