"use client";

import type { ReactNode } from "react";
import { IconGitPullRequest } from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Popover, PopoverAnchor, PopoverContent } from "@kandev/ui/popover";
import {
  IntegrationChangeRequestMultiStatusContent,
  IntegrationChangeRequestStatusContent,
} from "./integration-change-request-status-content";
import { CHANGE_REQUEST_CI_DESKTOP_SCROLL_CLASS } from "./change-request-ci-anatomy";
import {
  ChangeRequestStatusChipHoverArea,
  useChangeRequestStatusChipTriggerGuard,
} from "./change-request-status-chrome";
import {
  IntegrationChangeRequestStatusTrigger,
  NormalizedChangeRequestTopbarStatusIcon,
  normalizedTopbarColor,
} from "./integration-change-request-status-trigger";
import type { IntegrationChangeRequestStatusItem } from "./integration-change-request-status-types";
import { useIntegrationStatusHover, useRefreshItemsWhenOpen } from "./use-integration-status-hover";

function MultiStatusMenu({
  items,
  trigger,
  closePopover,
}: {
  items: readonly IntegrationChangeRequestStatusItem[];
  trigger: ReactNode;
  closePopover: () => void;
}) {
  return (
    <DropdownMenu onOpenChange={(open) => open && closePopover()}>
      <PopoverAnchor asChild>
        <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
      </PopoverAnchor>
      <DropdownMenuContent align="end" className="w-72">
        {items.map((item) => (
          <DropdownMenuItem
            key={item.id}
            className="cursor-pointer gap-2"
            onSelect={item.onOpenReview}
          >
            <IconGitPullRequest
              className={`h-4 w-4 shrink-0 ${normalizedTopbarColor(item, item.status ?? "neutral")}`}
            />
            <div className="flex min-w-0 flex-1 flex-col">
              <span className="text-xs font-medium">
                {item.repositoryLabel ?? "Pull request"} #{item.number}
              </span>
              <span className="truncate text-[11px] text-muted-foreground">{item.title}</span>
            </div>
            <NormalizedChangeRequestTopbarStatusIcon
              item={item}
              status={item.status ?? "neutral"}
            />
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function StatusTriggerMount({
  items,
  surface,
  trigger,
  hover,
}: {
  items: readonly IntegrationChangeRequestStatusItem[];
  surface: "topbar" | "composer";
  trigger: ReactNode;
  hover: ReturnType<typeof useIntegrationStatusHover>;
}) {
  if (surface === "composer") {
    return (
      <ChangeRequestStatusChipHoverArea handlers={hover}>
        <PopoverAnchor asChild>{trigger}</PopoverAnchor>
      </ChangeRequestStatusChipHoverArea>
    );
  }
  if (items.length === 1) return <PopoverAnchor asChild>{trigger}</PopoverAnchor>;
  return (
    <MultiStatusMenu
      items={items}
      trigger={trigger}
      closePopover={() => hover.onOpenChange(false)}
    />
  );
}

export function DesktopIntegrationChangeRequestStatus({
  items,
  surface,
  hoverDisabled = false,
}: {
  items: readonly IntegrationChangeRequestStatusItem[];
  surface: "topbar" | "composer";
  hoverDisabled?: boolean;
}) {
  const hover = useIntegrationStatusHover(hoverDisabled);
  const chipGuard = useChangeRequestStatusChipTriggerGuard();
  const [single] = items;
  useRefreshItemsWhenOpen(hover.open, items);
  const trigger = (
    <IntegrationChangeRequestStatusTrigger
      ref={surface === "composer" ? chipGuard.ref : undefined}
      items={items}
      mobile={false}
      surface={surface}
      hover={surface === "topbar" ? hover : undefined}
      onClick={
        surface === "topbar" && items.length === 1
          ? () => {
              single.onOpenReview();
              hover.onOpenChange(false);
            }
          : undefined
      }
    />
  );
  return (
    <Popover open={hover.open} onOpenChange={hover.onOpenChange}>
      <StatusTriggerMount items={items} surface={surface} trigger={trigger} hover={hover} />
      <PopoverContent
        data-testid="integration-change-request-status-popover"
        align={surface === "composer" ? "start" : "end"}
        side={surface === "composer" ? "top" : "bottom"}
        sideOffset={surface === "composer" ? 8 : 4}
        className={`${items.length === 1 ? "w-80" : "w-96"} ${surface === "composer" ? "p-2.5" : ""} ${CHANGE_REQUEST_CI_DESKTOP_SCROLL_CLASS}`}
        onMouseEnter={hover.onContentEnter}
        onMouseMove={hover.onContentEnter}
        onMouseLeave={hover.onContentLeave}
        onPointerDownOutside={surface === "composer" ? chipGuard.onPointerDownOutside : undefined}
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        {items.length === 1 ? (
          <IntegrationChangeRequestStatusContent item={single} mobile={false} contained={false} />
        ) : (
          <IntegrationChangeRequestMultiStatusContent
            items={items}
            mobile={false}
            contained={false}
          />
        )}
      </PopoverContent>
    </Popover>
  );
}
