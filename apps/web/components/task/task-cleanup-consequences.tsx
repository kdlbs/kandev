import { cn } from "@/lib/utils";
import type { CleanupSummary } from "./task-cleanup-summary";

type TaskCleanupConsequencesProps = {
  summary: CleanupSummary;
  compact?: boolean;
};

export function TaskCleanupConsequences({
  summary,
  compact = false,
}: TaskCleanupConsequencesProps) {
  const effectsClass = cn(
    "min-w-0 [overflow-wrap:anywhere]",
    compact ? "space-y-0.5" : "list-disc space-y-1.5 pl-5",
  );
  const notesClass = cn(
    "min-w-0 text-muted-foreground [overflow-wrap:anywhere]",
    compact ? "space-y-0.5" : "space-y-1",
  );

  if (compact) {
    return (
      <span data-testid="task-cleanup-consequences" className="block min-w-0 space-y-1">
        <span data-testid="task-cleanup-effects" role="list" className={effectsClass}>
          {summary.effects.map((effect, index) => (
            <span key={index} role="listitem" className="block">
              {effect}
            </span>
          ))}
        </span>
        {summary.notes.length > 0 && (
          <span data-testid="task-cleanup-notes" className={cn("block", notesClass)}>
            {summary.notes.map((note, index) => (
              <span key={index} className="block" data-testid="task-cleanup-note">
                {note}
              </span>
            ))}
          </span>
        )}
      </span>
    );
  }

  return (
    <div data-testid="task-cleanup-consequences" className="min-w-0 space-y-2">
      <ul data-testid="task-cleanup-effects" className={effectsClass}>
        {summary.effects.map((effect, index) => (
          <li key={index} data-testid="task-cleanup-effect">
            {effect}
          </li>
        ))}
      </ul>
      {summary.notes.length > 0 && (
        <div data-testid="task-cleanup-notes" className={notesClass}>
          {summary.notes.map((note, index) => (
            <p key={index} data-testid="task-cleanup-note">
              {note}
            </p>
          ))}
        </div>
      )}
    </div>
  );
}
