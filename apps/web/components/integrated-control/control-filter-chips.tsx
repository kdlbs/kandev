import { cn } from "@/lib/utils";

export type FilterChip<T extends string> = { value: T; label: string; count?: number };

/** Pill toggles that select one filter; the count, when given, follows the label. */
export function ControlFilterChips<T extends string>({
  chips,
  selected,
  onSelect,
  testId,
}: {
  chips: FilterChip<T>[];
  selected: T;
  onSelect: (value: T) => void;
  testId?: string;
}) {
  return (
    <div className="flex flex-wrap gap-1.5" data-testid={testId}>
      {chips.map((chip) => {
        const active = chip.value === selected;
        return (
          <button
            key={chip.value}
            type="button"
            aria-pressed={active}
            onClick={() => onSelect(chip.value)}
            className={cn(
              "min-h-7 cursor-pointer rounded-full border px-2.5 text-xs [@media(pointer:coarse)]:min-h-11",
              active
                ? "border-primary bg-primary/10 text-primary"
                : "border-border text-muted-foreground hover:bg-muted",
            )}
            data-testid={testId ? `${testId}-${chip.value}` : undefined}
          >
            {chip.label}
            {chip.count !== undefined && <span className="ml-1 tabular-nums">{chip.count}</span>}
          </button>
        );
      })}
    </div>
  );
}
