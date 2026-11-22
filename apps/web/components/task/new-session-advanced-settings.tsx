"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { cn } from "@/lib/utils";
import { NewSessionMCPPicker, type NewSessionMCPPickerProps } from "./new-session-mcp-picker";

export function NewSessionAdvancedSettings(props: NewSessionMCPPickerProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="min-w-0">
      <CollapsibleTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          className={controlSizingClassName(
            "standard",
            "w-full justify-start gap-1 px-1 text-[11px] text-muted-foreground/70 hover:bg-transparent hover:text-muted-foreground cursor-pointer",
          )}
          data-testid="new-session-advanced-settings-trigger"
        >
          <span>{t("task:advancedSettings")}</span>
          <IconChevronDown
            className={cn("size-3 motion-safe:transition-transform", open && "rotate-180")}
            aria-hidden="true"
          />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="min-w-0 px-1 pt-1">
        <NewSessionMCPPicker {...props} />
      </CollapsibleContent>
    </Collapsible>
  );
}
