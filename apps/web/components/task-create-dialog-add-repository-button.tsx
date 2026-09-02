"use client";

import { IconPlus } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";

export function AddRepositoryButton({
  canAddMore,
  addHint,
  addLabel,
  onAdd,
}: {
  canAddMore: boolean;
  addHint?: string;
  addLabel?: string;
  onAdd: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex" tabIndex={canAddMore ? undefined : 0}>
          <button
            type="button"
            onClick={onAdd}
            disabled={!canAddMore}
            aria-label={t("task:addRepository")}
            data-testid="add-repository"
            className={cn(
              "inline-flex items-center justify-center gap-1.5 rounded-md text-muted-foreground",
              addLabel ? "h-11 px-2 text-xs md:h-9" : "h-11 w-11 md:h-7 md:w-7",
              canAddMore
                ? "hover:bg-muted hover:text-foreground cursor-pointer"
                : "opacity-40 cursor-not-allowed",
            )}
          >
            <IconPlus className="h-3.5 w-3.5" />
            {addLabel ? <span>{addLabel}</span> : null}
          </button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{addHint ?? t("task:addAnotherRepository")}</TooltipContent>
    </Tooltip>
  );
}
