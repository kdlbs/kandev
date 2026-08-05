"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

export function ProfileEnabledHelp() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  return (
    <Tooltip open={open} onOpenChange={setOpen}>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="size-11 shrink-0 cursor-help text-muted-foreground sm:size-7"
          aria-label={t("agents:enabledProfileInfo")}
          onClick={() => setOpen((current) => !current)}
          data-testid="profile-enabled-help"
        >
          <IconInfoCircle className="size-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs text-xs leading-relaxed">
        {t("agents:enabledProfileHelper")}
      </TooltipContent>
    </Tooltip>
  );
}
