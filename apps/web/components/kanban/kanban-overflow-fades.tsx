import { IconChevronDown, IconChevronUp } from "@tabler/icons-react";
import { cn } from "@kandev/ui/lib/utils";
import type {
  KanbanOverflowAxis,
  KanbanOverflowState,
} from "@/hooks/domains/kanban/use-kanban-overflow";

type KanbanOverflowFadesProps = {
  axis: KanbanOverflowAxis;
  state: KanbanOverflowState;
};

const VISIBLE_FADE_CLASS = "kanban-overflow-fade-visible";

export function KanbanOverflowFades({ axis, state }: KanbanOverflowFadesProps) {
  const showVertical = axis === "vertical" || axis === "both";
  const showHorizontal = axis === "horizontal" || axis === "both";

  return (
    <>
      {showVertical && (
        <div
          aria-hidden="true"
          className={cn(
            "kanban-overflow-fade kanban-overflow-fade-top",
            state.canScrollTop && VISIBLE_FADE_CLASS,
          )}
          data-testid="kanban-overflow-top-fade"
          data-visible={state.canScrollTop}
        >
          <IconChevronUp className="kanban-overflow-chevron" size={18} stroke={2.5} />
        </div>
      )}
      {showVertical && (
        <div
          aria-hidden="true"
          className={cn(
            "kanban-overflow-fade kanban-overflow-fade-bottom",
            state.canScrollBottom && VISIBLE_FADE_CLASS,
          )}
          data-testid="kanban-overflow-bottom-fade"
          data-visible={state.canScrollBottom}
        >
          <IconChevronDown className="kanban-overflow-chevron" size={18} stroke={2.5} />
        </div>
      )}
      {showHorizontal && (
        <div
          aria-hidden="true"
          className={cn(
            "kanban-overflow-fade kanban-overflow-fade-left",
            state.canScrollLeft && VISIBLE_FADE_CLASS,
          )}
          data-testid="kanban-overflow-left-fade"
          data-visible={state.canScrollLeft}
        />
      )}
      {showHorizontal && (
        <div
          aria-hidden="true"
          className={cn(
            "kanban-overflow-fade kanban-overflow-fade-right",
            state.canScrollRight && VISIBLE_FADE_CLASS,
          )}
          data-testid="kanban-overflow-right-fade"
          data-visible={state.canScrollRight}
        />
      )}
    </>
  );
}
