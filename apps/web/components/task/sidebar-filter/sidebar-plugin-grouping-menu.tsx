"use client";

import { useTranslation } from "react-i18next";
import {
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@kandev/ui/dropdown-menu";
import { useSidebarGroupings } from "@/hooks/use-sidebar-groupings";

export function SidebarPluginGroupingMenu({ separator = true }: { separator?: boolean } = {}) {
  const { t } = useTranslation();
  const { groupings, selectGrouping } = useSidebarGroupings();
  if (!groupings.length) return null;
  return (
    <>
      {separator && <DropdownMenuSeparator />}
      <DropdownMenuGroup aria-label={t("sidebar:pluginGroupingSection")}>
        <DropdownMenuLabel>{t("sidebar:pluginGroupingSection")}</DropdownMenuLabel>
        {groupings.map((choice) => (
          <DropdownMenuItem
            key={choice.reference}
            disabled={Boolean(choice.disabledReason)}
            data-testid="sidebar-plugin-grouping"
            data-grouping-reference={choice.reference}
            title={choice.disabledReason ?? choice.label}
            onSelect={(event) => {
              const request = selectGrouping(choice);
              if (request === undefined) event.preventDefault();
              else void request;
            }}
            className="min-h-7 cursor-pointer text-xs [@media(any-pointer:coarse)]:min-h-[44px] max-md:min-h-[44px]"
          >
            <span className="min-w-0 truncate">{choice.label}</span>
            {choice.disabledReason && (
              <span className="ml-auto text-[10px] text-muted-foreground">
                {choice.disabledReason}
              </span>
            )}
          </DropdownMenuItem>
        ))}
      </DropdownMenuGroup>
    </>
  );
}
