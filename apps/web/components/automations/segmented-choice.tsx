"use client";

import { useState } from "react";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

export type SegmentedOption<V extends string> = {
  value: V;
  label: string;
  description: string;
};

type SegmentedChoiceProps<V extends string> = {
  /** Prefix for the heading, description, and option element IDs. */
  id: string;
  label: string;
  value: V;
  options: readonly SegmentedOption<V>[];
  onValueChange: (value: V) => void;
  isDirty?: boolean;
  testId?: string;
};

// Phones stack the options so long labels stay readable; coarse pointers get
// 44 px targets.
const GROUP_CLASS_NAME =
  "w-full rounded-md border bg-muted p-0.5 max-sm:flex-col max-sm:items-stretch";
const ITEM_CLASS_NAME =
  "min-w-0 flex-1 cursor-pointer whitespace-normal rounded-[5px] px-3 text-sm text-muted-foreground data-[state=on]:bg-card data-[state=on]:font-medium data-[state=on]:text-foreground data-[state=on]:shadow-sm max-md:min-h-11 [@media(pointer:coarse)]:min-h-11";

/**
 * A compact single choice: short option labels side by side, one line that
 * describes the selected option, and a tooltip that describes each other one.
 */
export function SegmentedChoice<V extends string>({
  id,
  label,
  value,
  options,
  onValueChange,
  isDirty,
  testId,
}: SegmentedChoiceProps<V>) {
  const headingId = `${id}-heading`;
  const descriptionId = `${id}-description`;
  const selected = options.find((option) => option.value === value);
  // The selected option is already described below the group, so only the
  // other options open a tooltip. Touch devices rely on that line alone, since
  // a tap selects. A tooltip opened under an earlier selection stays closed.
  const { isFinePointer } = useResponsiveBreakpoint();
  const [tooltip, setTooltip] = useState<{ option: V; selected: V } | null>(null);
  return (
    <div className="space-y-1.5">
      <h3
        id={headingId}
        className="text-xs font-medium uppercase tracking-wider text-muted-foreground"
      >
        {label}
      </h3>
      <ToggleGroup
        type="single"
        spacing={1}
        value={value}
        onValueChange={(next) => {
          // Pressing the selected option again would clear the choice.
          if (next) onValueChange(next as V);
        }}
        aria-labelledby={headingId}
        aria-describedby={descriptionId}
        data-settings-dirty={isDirty}
        data-testid={testId}
        className={GROUP_CLASS_NAME}
      >
        {options.map((option) => (
          <Tooltip
            key={option.value}
            open={
              isFinePointer &&
              tooltip?.option === option.value &&
              tooltip.selected === value &&
              option.value !== value
            }
            onOpenChange={(open) =>
              setTooltip(open ? { option: option.value, selected: value } : null)
            }
          >
            <TooltipTrigger asChild>
              <ToggleGroupItem
                value={option.value}
                id={`${id}-${option.value.replace(/_/g, "-")}`}
                className={ITEM_CLASS_NAME}
              >
                {option.label}
              </ToggleGroupItem>
            </TooltipTrigger>
            <TooltipContent className="max-w-[280px]">{option.description}</TooltipContent>
          </Tooltip>
        ))}
      </ToggleGroup>
      <p id={descriptionId} className="text-xs text-muted-foreground">
        {selected?.description}
      </p>
    </div>
  );
}
