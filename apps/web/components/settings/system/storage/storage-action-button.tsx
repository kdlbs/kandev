import type { ComponentProps } from "react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { settingsActionClassName } from "@/components/settings/settings-control";

type Props = ComponentProps<typeof Button> & { disabledReason?: string };

export function StorageActionButton({ disabledReason, disabled, className, ...props }: Props) {
  const isDisabled = disabled || Boolean(disabledReason);
  const button = (
    <Button {...props} disabled={isDisabled} className={settingsActionClassName(className)} />
  );
  if (!disabledReason) return button;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={0} className="inline-flex">
          {button}
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">{disabledReason}</TooltipContent>
    </Tooltip>
  );
}
