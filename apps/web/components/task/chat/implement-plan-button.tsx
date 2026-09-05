"use client";

import type { ReactNode } from "react";
import { IconChevronDown, IconPlus, IconRocket } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@kandev/ui/lib/utils";
import { useTranslation } from "react-i18next";

/**
 * Split-button: primary "Implement" runs the plan inline in the current
 * session; the dropdown caret offers "Implement in fresh agent" which starts
 * a new session with a clean context window. `onClick(fresh)` is invoked
 * with `false` for the primary path and `true` for the fresh-agent path.
 */
type ImplementPlanButtonProps = {
  onClick: (fresh: boolean) => void | Promise<unknown>;
  presentation?: "desktop" | "mobile";
  disabled?: boolean;
  disabledReason?: string;
  framed?: boolean;
  testIds?: {
    root?: string;
    button?: string;
    menuTrigger?: string;
    freshItem?: string;
  };
};

type ImplementPlanPresentation = "desktop" | "mobile";

function legacyRootTestId(rootId: string) {
  return rootId === "implement-plan-control" ? undefined : "implement-plan-control";
}

function controlSizeClass(presentation: ImplementPlanPresentation, framed?: boolean): string {
  if (presentation === "mobile") return "min-h-11 min-w-11";
  if (framed) return "h-5";
  return "h-7";
}

function disabledInteractionClass(disabled?: boolean): string {
  return disabled ? "pointer-events-none cursor-not-allowed" : "cursor-pointer";
}

function framedRootClass(framed?: boolean): string | undefined {
  return framed
    ? "border border-violet-400/35 bg-background/40 focus-within:ring-[2px] focus-within:ring-violet-400/25"
    : undefined;
}

function DisabledImplementTooltip({
  disabledReason,
  children,
}: {
  disabledReason: string;
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={0} className="inline-flex">
          {children}
        </span>
      </TooltipTrigger>
      <TooltipContent>{disabledReason}</TooltipContent>
    </Tooltip>
  );
}

export function ImplementPlanButton({
  onClick,
  presentation = "desktop",
  disabled,
  disabledReason,
  framed,
  testIds,
}: ImplementPlanButtonProps) {
  const ids = {
    root: testIds?.root ?? "implement-plan-control",
    button: testIds?.button ?? "implement-plan-button",
    menuTrigger: testIds?.menuTrigger ?? "implement-plan-menu-trigger",
    freshItem: testIds?.freshItem ?? "implement-fresh-menu-item",
  };
  const controlSize = controlSizeClass(presentation, framed);
  const splitButton = (
    <div
      data-testid={ids.root}
      data-legacy-testid={legacyRootTestId(ids.root)}
      className={cn("inline-flex items-center rounded-md", framedRootClass(framed))}
    >
      <ImplementPrimaryButton
        buttonId={ids.button}
        controlSize={controlSize}
        disabled={disabled}
        disabledReason={disabledReason}
        onClick={onClick}
      />
      <ImplementMenuButton
        controlSize={controlSize}
        disabled={disabled}
        freshItemId={ids.freshItem}
        menuTriggerId={ids.menuTrigger}
        onClick={onClick}
      />
    </div>
  );

  if (!disabledReason) return splitButton;

  return (
    <DisabledImplementTooltip disabledReason={disabledReason}>
      {splitButton}
    </DisabledImplementTooltip>
  );
}

function ImplementPrimaryButton({
  buttonId,
  controlSize,
  disabled,
  disabledReason,
  onClick,
}: {
  buttonId: string;
  controlSize: string;
  disabled?: boolean;
  disabledReason?: string;
  onClick: (fresh: boolean) => void | Promise<unknown>;
}) {
  const { t } = useTranslation();
  const button = (
    <span className="inline-flex">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        data-testid={buttonId}
        disabled={disabled}
        className={cn(
          "gap-1.5 px-2 text-violet-400 rounded-r-none pr-1.5 border-transparent",
          controlSize,
          "hover:bg-muted/40 focus-visible:border-transparent focus-visible:ring-violet-400/30",
          disabledInteractionClass(disabled),
        )}
        onClick={() => onClick(false)}
      >
        <IconRocket className="h-4 w-4" />
        <span className="text-xs">{t("task:implement")}</span>
      </Button>
    </span>
  );
  if (disabledReason) return button;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent>{t("task:implementThePlanInThisSession")}</TooltipContent>
    </Tooltip>
  );
}

function ImplementMenuButton({
  controlSize,
  disabled,
  freshItemId,
  menuTriggerId,
  onClick,
}: {
  controlSize: string;
  disabled?: boolean;
  freshItemId: string;
  menuTriggerId: string;
  onClick: (fresh: boolean) => void | Promise<unknown>;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          data-testid={menuTriggerId}
          aria-label={t("task:moreImplementOptions")}
          aria-disabled={disabled}
          disabled={disabled}
          className={cn(
            "px-1 text-violet-400 rounded-l-none border-y-0 border-r-0 border-l border-violet-400/20",
            controlSize,
            "hover:bg-muted/40 focus-visible:border-y-0 focus-visible:border-r-0 focus-visible:border-l-violet-400/20 focus-visible:ring-0",
            disabledInteractionClass(disabled),
          )}
        >
          <IconChevronDown className="h-3.5 w-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuItem
          data-testid={freshItemId}
          onClick={() => onClick(true)}
          className="cursor-pointer"
        >
          <IconPlus className="h-4 w-4 mr-2 shrink-0 self-start mt-0.5" />
          <div>
            <div>{t("task:implementInFreshAgent")}</div>
            <div className="text-[11px] text-muted-foreground font-normal">
              {t("task:startsANewSessionWithA")}
            </div>
          </div>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
