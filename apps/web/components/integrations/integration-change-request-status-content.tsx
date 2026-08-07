"use client";

import {
  IconCircleCheckFilled,
  IconCircleXFilled,
  IconLoader2,
  IconPointFilled,
} from "@tabler/icons-react";
import { DrawerClose } from "@kandev/ui/drawer";
import { Button } from "@kandev/ui/button";
import { t } from "@/lib/i18n";
import {
  ChangeRequestCIPopoverFrame,
  ChangeRequestChecksSection,
  ChangeRequestCommentsRow,
  ChangeRequestPopoverFooter,
  ChangeRequestPopoverHeader,
  ChangeRequestReviewRow,
  type ChangeRequestCheckCounts,
} from "./change-request-ci-anatomy";
import type {
  IntegrationChangeRequestPipelineState,
  IntegrationChangeRequestStatusItem,
} from "./integration-change-request-status-types";

export function PipelineStatusGlyph({ state }: { state: IntegrationChangeRequestPipelineState }) {
  if (state === "success") {
    return <IconCircleCheckFilled className="h-3.5 w-3.5 text-green-500" aria-hidden="true" />;
  }
  if (state === "failure") {
    return <IconCircleXFilled className="h-3.5 w-3.5 text-red-500" aria-hidden="true" />;
  }
  if (state === "pending") {
    return (
      <IconLoader2
        className="h-3.5 w-3.5 text-yellow-500 animate-spin [animation-duration:3s]"
        aria-hidden="true"
      />
    );
  }
  return <IconPointFilled className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />;
}

function countsFor(item: IntegrationChangeRequestStatusItem): ChangeRequestCheckCounts {
  const counts = { passed: 0, pending: 0, failed: 0 };
  item.pipelineRows?.forEach((row) => {
    if (row.state === "success") counts.passed += 1;
    else if (row.state === "pending") counts.pending += 1;
    else if (row.state === "failure") counts.failed += 1;
  });
  return counts;
}

function StatusBody({ item }: { item: IntegrationChangeRequestStatusItem }) {
  if (item.error) {
    return <div className="px-1 py-2 text-xs text-destructive">{item.error}</div>;
  }
  return (
    <ChangeRequestChecksSection
      counts={countsFor(item)}
      rows={item.pipelineRows ?? []}
      loading={item.loading}
    />
  );
}

export function IntegrationChangeRequestStatusContent({
  item,
  mobile,
  contained = true,
}: {
  item: IntegrationChangeRequestStatusItem;
  mobile: boolean;
  contained?: boolean;
}) {
  const content = <StatusFrame item={item} />;
  if (!contained) return content;
  return (
    <div
      data-testid="integration-change-request-status-scroll-body"
      className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden overscroll-contain p-2.5"
      data-vaul-no-drag={mobile ? "" : undefined}
    >
      {content}
    </div>
  );
}

function StatusFrame({ item }: { item: IntegrationChangeRequestStatusItem }) {
  return (
    <ChangeRequestCIPopoverFrame>
      <ChangeRequestPopoverHeader
        number={item.number}
        title={item.title}
        url={item.url}
        onOpenReview={item.onOpenReview}
      />
      <StatusBody item={item} />
      {item.review || item.unresolvedComments ? (
        <div className="flex flex-col gap-0">
          {item.review ? <ChangeRequestReviewRow {...item.review} /> : null}
          <ChangeRequestCommentsRow count={item.unresolvedComments ?? 0} />
        </div>
      ) : null}
      <ChangeRequestPopoverFooter updatedAt={item.updatedAt} />
    </ChangeRequestCIPopoverFrame>
  );
}

export function IntegrationChangeRequestMultiStatusContent({
  items,
  mobile,
  contained = true,
}: {
  items: readonly IntegrationChangeRequestStatusItem[];
  mobile: boolean;
  contained?: boolean;
}) {
  return (
    <div
      data-testid={contained ? "integration-change-request-status-scroll-body" : undefined}
      className={
        contained
          ? "min-h-0 flex-1 overflow-y-auto overflow-x-hidden overscroll-contain p-2.5"
          : "min-h-0"
      }
      data-vaul-no-drag={mobile ? "" : undefined}
    >
      {items.map((item) => (
        <section key={item.id} className="mb-3 border-b pb-3 last:mb-0 last:border-b-0 last:pb-0">
          <ChangeRequestCIPopoverFrame>
            <ChangeRequestPopoverHeader
              number={item.number}
              title={item.title}
              url={item.url}
              onOpenReview={item.onOpenReview}
            />
            <StatusBody item={item} />
            {item.review || item.unresolvedComments ? (
              <div className="flex flex-col gap-0">
                {item.review ? <ChangeRequestReviewRow {...item.review} /> : null}
                <ChangeRequestCommentsRow count={item.unresolvedComments ?? 0} />
              </div>
            ) : null}
            <ChangeRequestPopoverFooter updatedAt={item.updatedAt} />
          </ChangeRequestCIPopoverFrame>
          {mobile ? (
            <DrawerClose asChild>
              <Button
                type="button"
                variant="outline"
                className="mt-2 h-11 w-full cursor-pointer"
                onClick={item.onOpenReview}
              >
                {t("integrations:openReviewNumber", { number: item.number })}
              </Button>
            </DrawerClose>
          ) : null}
        </section>
      ))}
    </div>
  );
}
