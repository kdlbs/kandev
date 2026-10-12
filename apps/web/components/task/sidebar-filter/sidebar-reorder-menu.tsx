"use client";

import { IconDots } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useTranslation } from "react-i18next";

export function SidebarReorderMenu({
  label,
  position,
  count,
  onMove,
  isDrawerLayout,
  testId,
  moveUpLabelKey,
  moveDownLabelKey,
}: {
  label: string;
  position: number;
  count: number;
  onMove: (offset: -1 | 1) => void;
  isDrawerLayout: boolean;
  testId: string;
  moveUpLabelKey?: string;
  moveDownLabelKey?: string;
}) {
  const { t } = useTranslation();
  const triggerSize = isDrawerLayout
    ? "size-11"
    : "size-7 max-md:size-11 [@media(pointer:coarse)]:size-11";
  const itemSize = isDrawerLayout ? "min-h-11" : "min-h-7 [@media(pointer:coarse)]:min-h-11";

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={`${triggerSize} min-h-7 cursor-pointer text-muted-foreground hover:text-foreground`}
          aria-label={t("task:sidebarReorderMore", { label })}
          data-testid={testId}
        >
          <IconDots className="size-4" aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40">
        <DropdownMenuItem
          className={`${itemSize} cursor-pointer`}
          disabled={position <= 1}
          onSelect={() => onMove(-1)}
          data-testid={`${testId}-move-up`}
        >
          {t(moveUpLabelKey ?? "task:sidebarReorderMoveUp", { position })}
        </DropdownMenuItem>
        <DropdownMenuItem
          className={`${itemSize} cursor-pointer`}
          disabled={position >= count}
          onSelect={() => onMove(1)}
          data-testid={`${testId}-move-down`}
        >
          {t(moveDownLabelKey ?? "task:sidebarReorderMoveDown", { position })}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
