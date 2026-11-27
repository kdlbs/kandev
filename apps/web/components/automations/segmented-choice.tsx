"use client";

import { useState } from "react";
import { RadioGroup } from "radix-ui";
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
// 44 px targets. The selected option carries a primary fill and outline, which
// stays visible against the muted group in both themes. It is styled through
// aria-checked because the tooltip trigger replaces the item's data-state.
// It is a radio group, so the arrow keys move the choice itself.
const GROUP_CLASS_NAME =
  "flex w-full items-center gap-1 rounded-md border bg-muted p-0.5 max-sm:flex-col max-sm:items-stretch";
const ITEM_CLASS_NAME =
  "inline-flex min-h-7 min-w-0 flex-1 cursor-pointer items-center justify-center whitespace-normal rounded-[5px] px-3 text-sm outline-none transition-colors hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 text-muted-foreground aria-checked:bg-primary/15 aria-checked:font-medium aria-checked:text-foreground aria-checked:shadow-[inset_0_0_0_1px_var(--primary)] aria-checked:hover:bg-primary/20 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11";

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
  const { isFinePointer, isMobile: isPhone } = useResponsiveBreakpoint();
  const [tooltip, setTooltip] = useState<{ option: V; selected: V } | null>(null);
  return (
    <div className="space-y-1.5">
      <h3
        id={headingId}
        className="text-xs font-medium uppercase tracking-wider text-muted-foreground"
      >
        {label}
      </h3>
      <RadioGroup.Root
        value={value}
        onValueChange={(next) => onValueChange(next as V)}
        orientation={isPhone ? "vertical" : "horizontal"}
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
              <RadioGroup.Item
                value={option.value}
                id={`${id}-${option.value.replace(/_/g, "-")}`}
                className={ITEM_CLASS_NAME}
              >
                {option.label}
              </RadioGroup.Item>
            </TooltipTrigger>
            <TooltipContent className="max-w-[280px]">{option.description}</TooltipContent>
          </Tooltip>
        ))}
      </RadioGroup.Root>
      <p id={descriptionId} className="text-xs text-muted-foreground">
        {selected?.description}
      </p>
    </div>
  );
}
