"use client";

import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { SettingsInfo } from "@/components/settings/settings-info";
import { MCPSelectionContent } from "@/components/mcp/mcp-selection-picker";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useTaskCreateDialogPopoverContainer } from "@/hooks/use-task-create-dialog-popover-container";
import type { MCPInheritedSelection, MCPServerDefinition } from "@/lib/types/http-mcp";

type Props = {
  definitions: MCPServerDefinition[];
  loading: boolean;
  selectedIds: string[];
  onSelectedIdsChange: (ids: string[]) => void;
  inherited: MCPInheritedSelection[];
  disabled?: boolean;
};

export function TaskCreateMCPSettingRow({ loading, ...selection }: Props) {
  const { t } = useTranslation();
  const id = useId();
  const touch = useTouchDrawer();
  const portalContainer = useTaskCreateDialogPopoverContainer();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const title = t("settings:mcpServers");
  const description = t("task:mcpTaskSelectionInfo");
  const trigger = (
    <Button
      id={id}
      type="button"
      variant="outline"
      aria-label={title}
      aria-haspopup="dialog"
      aria-expanded={open}
      data-testid="task-create-mcp-select"
      disabled={loading || selection.disabled}
      className={controlSizingClassName(
        "standard",
        "w-32 justify-between border-border/60 bg-muted/30 hover:bg-muted/60",
      )}
    >
      <span className="min-w-0 truncate tabular-nums">
        {loading
          ? t("common:loading")
          : t("settings:mcpSelectedCount", { count: selection.selectedIds.length })}
      </span>
      <IconChevronDown className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
    </Button>
  );
  const body = (
    <MCPSelectionContent
      {...selection}
      query={query}
      onQueryChange={setQuery}
      sectionLabel={t("settings:mcpAdditionalServers")}
    />
  );
  return (
    <div className="min-w-0" data-testid="task-create-mcp-setting-row">
      <div className="flex min-w-0 items-center gap-2" data-testid="task-create-mcp-selection">
        <div className="flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
          <label htmlFor={id}>{title}</label>
          <SettingsInfo label={title} testId="task-create-mcp-info">
            {description}
          </SettingsInfo>
        </div>
        {touch ? (
          <Drawer open={open} onOpenChange={setOpen}>
            <DrawerTrigger asChild>{trigger}</DrawerTrigger>
            <DrawerContent
              data-testid="task-create-mcp-options"
              className="h-[min(80dvh,42rem)] !max-h-[80dvh] overflow-hidden pb-[env(safe-area-inset-bottom)]"
            >
              <DrawerHeader className="shrink-0 text-left">
                <div className="flex items-center justify-between gap-3">
                  <DrawerTitle>{title}</DrawerTitle>
                  <DrawerClose asChild>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className={controlSizingClassName("icon")}
                      aria-label={t("common:close")}
                    >
                      <IconX className="size-4" aria-hidden="true" />
                    </Button>
                  </DrawerClose>
                </div>
                <DrawerDescription>{description}</DrawerDescription>
              </DrawerHeader>
              <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 pb-4">
                {body}
              </div>
            </DrawerContent>
          </Drawer>
        ) : (
          <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger asChild>{trigger}</PopoverTrigger>
            <PopoverContent
              align="start"
              portalContainer={portalContainer}
              aria-label={title}
              data-testid="task-create-mcp-options"
              className="max-h-[min(28rem,var(--radix-popover-content-available-height))] w-[min(24rem,calc(100vw-2rem))] gap-3 overflow-hidden p-3"
            >
              <p className="shrink-0 text-xs text-pretty text-muted-foreground">{description}</p>
              <div className="min-h-0 overflow-y-auto overscroll-contain">{body}</div>
            </PopoverContent>
          </Popover>
        )}
      </div>
    </div>
  );
}
