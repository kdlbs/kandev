"use client";

import { IconMessagePlus, IconPlus, IconTerminal2 } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";

type QuickTabAddMenuProps = {
  onNewAgent: () => void;
  onNewTerminal: () => void;
};

/** Grouped creation menu for the shared Quick Chat surface. */
export function QuickTabAddMenu({ onNewAgent, onNewTerminal }: QuickTabAddMenuProps) {
  const { t } = useTranslation();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          size="icon"
          variant="ghost"
          className="h-11 w-11 shrink-0 cursor-pointer sm:h-6 sm:w-6"
          aria-label={t("sidebar:quickChatAdd")}
          data-testid="quick-chat-add-menu-trigger"
        >
          <IconPlus className="h-4 w-4 sm:h-3.5 sm:w-3.5" aria-hidden />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuLabel className="text-xs text-muted-foreground">
          {t("common:agents")}
        </DropdownMenuLabel>
        <DropdownMenuItem
          onSelect={onNewAgent}
          className="min-h-11 cursor-pointer gap-1.5 sm:min-h-7"
          data-testid="quick-chat-new-agent"
        >
          <IconMessagePlus className="h-3.5 w-3.5 shrink-0" aria-hidden />
          <span className="flex-1 truncate">{t("sidebar:quickChatNewAgent")}</span>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuLabel className="text-xs text-muted-foreground">
          {t("sidebar:quickChatTerminals")}
        </DropdownMenuLabel>
        <DropdownMenuItem
          onSelect={onNewTerminal}
          className="min-h-11 cursor-pointer gap-1.5 sm:min-h-7"
          data-testid="quick-chat-new-terminal"
        >
          <IconTerminal2 className="h-3.5 w-3.5 shrink-0" aria-hidden />
          <span className="flex-1 truncate">{t("sidebar:quickChatNewTerminal")}</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
