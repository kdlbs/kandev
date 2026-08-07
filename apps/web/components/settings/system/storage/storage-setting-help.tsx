"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconInfoCircle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";

export function StorageSettingHelp({ label, children }: { label: string; children: string }) {
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
          aria-label={t("system:storageMoreInformationAbout", { label })}
          onClick={() => setOpen((current) => !current)}
        >
          <IconInfoCircle className="size-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs text-xs leading-relaxed">{children}</TooltipContent>
    </Tooltip>
  );
}
