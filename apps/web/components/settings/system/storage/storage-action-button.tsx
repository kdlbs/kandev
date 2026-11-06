import type { ComponentProps } from "react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { cn } from "@/lib/utils";

type Props = ComponentProps<typeof Button> & {
  disabledReason?: string;
  focusId?: string;
  fullWidth?: boolean;
};

/** Keeps disabled tooltip wrappers aligned with opt-in full-width controls. */
export function StorageActionButton({
  disabledReason,
  disabled,
  className,
  focusId,
  fullWidth,
  ...props
}: Props) {
  const isDisabled = disabled || Boolean(disabledReason);
  const button = (
    <Button
      {...props}
      disabled={isDisabled}
      className={settingsActionClassName(cn(fullWidth && "w-full min-w-0", className))}
      data-storage-focus-id={focusId}
    />
  );
  if (!disabledReason) return button;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          tabIndex={0}
          className={cn("inline-flex", fullWidth && "w-full min-w-0")}
          data-storage-focus-id={focusId}
        >
          {button}
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">{disabledReason}</TooltipContent>
    </Tooltip>
  );
}
