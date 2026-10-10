"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { Repository } from "@/lib/types/http";

function returnFocus(trigger: HTMLButtonElement | null) {
  if (trigger?.disabled) {
    trigger.parentElement?.querySelector<HTMLButtonElement>("button:not([disabled])")?.focus();
  } else trigger?.focus();
}

export function QuickChatRepositoryAction({
  repositories,
  selectedIds,
  disabled,
  hint,
  onSelect,
}: {
  repositories: Repository[];
  selectedIds: string[];
  disabled: boolean;
  hint?: string;
  onSelect: (repositoryId: string) => void;
}) {
  const { t } = useTranslation();
  const touch = useTouchDrawer();
  const { isMobile } = useResponsiveBreakpoint();
  const usesSheet = touch || isMobile;
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const trigger = (
    <Button
      ref={triggerRef}
      type="button"
      variant="ghost"
      size="icon"
      disabled={disabled}
      aria-label={t("chat:addRepository")}
      aria-haspopup="dialog"
      aria-expanded={open}
      data-testid="add-repository"
      className={usesSheet ? "h-11 w-11 shrink-0" : "h-7 w-7 shrink-0"}
      onClick={() => setOpen(true)}
    >
      <IconPlus className="h-4 w-4" aria-hidden />
    </Button>
  );
  const picker = (
    <Command>
      <CommandInput placeholder={t("task:searchRepositories")} />
      <CommandList>
        <CommandEmpty>{t("chat:noRepositoriesAvailableInWorkspace")}</CommandEmpty>
        <CommandGroup>
          {repositories
            .filter((repo) => !selectedIds.includes(repo.id))
            .map((repo) => (
              <CommandItem
                key={repo.id}
                value={repo.id}
                keywords={[repo.name, repo.local_path ?? ""]}
                className={usesSheet ? "min-h-12" : undefined}
                onSelect={() => {
                  onSelect(repo.id);
                  setOpen(false);
                }}
              >
                {repo.name}
              </CommandItem>
            ))}
        </CommandGroup>
      </CommandList>
    </Command>
  );
  if (usesSheet) {
    return (
      <>
        {trigger}
        <MobilePickerSheet
          open={open}
          onOpenChange={setOpen}
          title={t("chat:addRepository")}
          contentTestId="quick-chat-repository-picker"
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            returnFocus(triggerRef.current);
          }}
        >
          {picker}
        </MobilePickerSheet>
      </>
    );
  }
  return (
    <Tooltip>
      <Popover open={open} onOpenChange={setOpen}>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>{trigger}</PopoverTrigger>
        </TooltipTrigger>
        <PopoverContent
          className="w-72 p-0"
          align="start"
          data-testid="quick-chat-repository-picker"
        >
          {picker}
        </PopoverContent>
      </Popover>
      <TooltipContent>{hint ?? t("chat:addRepository")}</TooltipContent>
    </Tooltip>
  );
}
