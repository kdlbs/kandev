"use client";

import { forwardRef, type ComponentPropsWithoutRef } from "react";
import {
  IconCheck,
  IconChevronDown,
  IconClock,
  IconGitPullRequest,
  IconChecklist,
  IconX,
} from "@tabler/icons-react";
import {
  ChangeRequestStatusChip,
  ChangeRequestTopbarButton,
  ChangeRequestTopbarContent,
} from "./change-request-status-chrome";
import { PipelineStatusGlyph } from "./integration-change-request-status-content";
import type {
  IntegrationChangeRequestPipelineState,
  IntegrationChangeRequestStatusItem,
} from "./integration-change-request-status-types";
import type { useIntegrationStatusHover } from "./use-integration-status-hover";

export function aggregatePipelineStatus(
  items: readonly IntegrationChangeRequestStatusItem[],
): IntegrationChangeRequestPipelineState {
  const statuses = items.map((item) => item.status ?? "neutral");
  if (statuses.includes("failure")) return "failure";
  if (statuses.includes("pending")) return "pending";
  if (statuses.length > 0 && statuses.every((status) => status === "success")) return "success";
  return "neutral";
}

export function normalizedTopbarColor(
  item: IntegrationChangeRequestStatusItem | null,
  status: IntegrationChangeRequestPipelineState,
) {
  if (item?.state === "merged") return "text-purple-500";
  if (item?.state === "closed" || status === "failure") return "text-red-500";
  if (item?.state === "draft" || status === "neutral") return "text-muted-foreground";
  if (status === "pending") return "text-yellow-500";
  return "text-green-500";
}

export function NormalizedChangeRequestTopbarStatusIcon({
  item,
  status,
}: {
  item: IntegrationChangeRequestStatusItem;
  status: IntegrationChangeRequestPipelineState;
}) {
  if (item.state === "merged") return <IconCheck className="h-3 w-3 text-purple-500" />;
  if (item.state === "closed") return <IconX className="h-3 w-3 text-muted-foreground" />;
  if (item.state === "draft") {
    return <IconGitPullRequest className="h-3 w-3 text-muted-foreground" />;
  }
  if (status === "failure") return <IconX className="h-3 w-3 text-red-500" />;
  if (status === "pending") return <IconClock className="h-3 w-3 text-yellow-500" />;
  if (status === "success") return <IconCheck className="h-3 w-3 text-green-500" />;
  return null;
}

type TriggerProps = {
  items: readonly IntegrationChangeRequestStatusItem[];
  mobile: boolean;
  surface: "topbar" | "composer";
  onClick?: () => void;
  hover?: ReturnType<typeof useIntegrationStatusHover>;
} & Omit<ComponentPropsWithoutRef<"button">, "children" | "onClick">;

function triggerHoverProps(hover: TriggerProps["hover"]) {
  if (!hover) return {};
  return {
    onMouseOver: hover.onTriggerEnter,
    onMouseEnter: hover.onTriggerEnter,
    onMouseMove: hover.onTriggerEnter,
    onPointerOver: hover.onTriggerEnter,
    onPointerEnter: hover.onTriggerEnter,
    onPointerMove: hover.onTriggerEnter,
    onMouseLeave: hover.onTriggerLeave,
    onPointerLeave: hover.onTriggerLeave,
    onFocus: hover.onTriggerEnter,
    onBlur: hover.onTriggerLeave,
  };
}

function TriggerContents({
  items,
  single,
  composer,
  status,
}: {
  items: readonly IntegrationChangeRequestStatusItem[];
  single: IntegrationChangeRequestStatusItem | null;
  composer: boolean;
  status: IntegrationChangeRequestPipelineState;
}) {
  if (!composer) {
    return (
      <ChangeRequestTopbarContent
        label={single ? `#${single.number}` : `${items.length} PRs`}
        colorClassName={normalizedTopbarColor(single, status)}
        statusIcon={
          single ? (
            <NormalizedChangeRequestTopbarStatusIcon item={single} status={status} />
          ) : undefined
        }
        dropdown={
          items.length > 1 ? (
            <IconChevronDown className="h-3 w-3 text-muted-foreground" />
          ) : undefined
        }
      />
    );
  }
  return (
    <>
      <IconChecklist className="h-3.5 w-3.5 text-muted-foreground" />
      <PipelineStatusGlyph state={status} />
      {items.length > 1 ? (
        <span className="text-[9px] font-semibold leading-none tabular-nums">{items.length}</span>
      ) : null}
    </>
  );
}

export const IntegrationChangeRequestStatusTrigger = forwardRef<HTMLButtonElement, TriggerProps>(
  function IntegrationChangeRequestStatusTrigger(
    { items, mobile, surface, onClick, hover, ...buttonProps },
    ref,
  ) {
    const single = items.length === 1 ? items[0] : null;
    const label = single ? `#${single.number} ${single.title}` : `${items.length} pull requests`;
    const status = single?.status ?? aggregatePipelineStatus(items);
    const composer = surface === "composer";
    const interactions = triggerHoverProps(hover);
    const contents = (
      <TriggerContents items={items} single={single} composer={composer} status={status} />
    );
    if (composer && !mobile) {
      return (
        <ChangeRequestStatusChip
          {...buttonProps}
          {...interactions}
          ref={ref}
          aria-label={label}
          data-testid="integration-change-request-status-chip"
          data-status={status}
          onClick={onClick}
        >
          {contents}
        </ChangeRequestStatusChip>
      );
    }
    return (
      <ChangeRequestTopbarButton
        {...buttonProps}
        {...interactions}
        ref={ref}
        className={mobile ? "h-11 cursor-pointer gap-1.5 px-2" : "cursor-pointer gap-1.5 px-2"}
        aria-label={label}
        data-testid={
          composer
            ? "integration-change-request-status-chip"
            : "integration-change-request-status-trigger"
        }
        onClick={onClick}
      >
        {contents}
      </ChangeRequestTopbarButton>
    );
  },
);
