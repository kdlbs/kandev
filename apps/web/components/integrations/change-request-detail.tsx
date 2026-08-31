"use client";

import type { ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { Separator } from "@kandev/ui/separator";
import { Spinner } from "@kandev/ui/spinner";
import { CollapsibleSection, PRMarkdownBody, formatTimeAgo } from "@/components/github/pr-shared";
import { t } from "@/lib/i18n";
import { ChangeRequestDetailHeader } from "./change-request-detail-header";
import { ChangeRequestReviews } from "./change-request-detail-reviews";
import { ChangeRequestChecks } from "./change-request-detail-checks";
import { ChangeRequestComments } from "./change-request-detail-comments";

export type ChangeRequestDetailPerson = {
  name: string;
  url?: string;
  avatarUrl?: string;
  isBot?: boolean;
  kind?: "user" | "team";
};

export type ChangeRequestDetailAction = {
  id: string;
  label: string;
  pendingLabel?: string;
  placement: "header" | "comment" | "thread";
  tone?: "default" | "success" | "danger" | "secondary";
  input?: "text";
  disabled?: boolean;
  busy?: boolean;
};

export type ChangeRequestDetailItemAction = Omit<ChangeRequestDetailAction, "placement" | "input">;

export type ChangeRequestDetailReview = {
  id: string;
  author: ChangeRequestDetailPerson;
  state: string;
  body?: string;
  createdAt?: string;
  actions?: ChangeRequestDetailItemAction[];
};

export type ChangeRequestDetailCheck = {
  id: string;
  name: string;
  state: string;
  conclusion?: string;
  url?: string;
  output?: string;
  startedAt?: string;
  completedAt?: string;
};

export type ChangeRequestDetailComment = {
  id: string;
  parentId?: string;
  author: ChangeRequestDetailPerson;
  body: string;
  createdAt?: string;
  url?: string;
  path?: string;
  line?: number;
  resolved?: boolean;
};

export type ChangeRequestDetailModel = {
  providerId: string;
  reviewKey: string;
  number: string | number;
  title: string;
  url: string;
  state: string;
  draft?: boolean;
  author: ChangeRequestDetailPerson;
  createdAt?: string;
  mergedAt?: string;
  closedAt?: string;
  sourceBranch: string;
  targetBranch: string;
  additions: number;
  deletions: number;
  description?: string;
  reviewState?: string;
  pendingReviewCount?: number;
  reviews: ChangeRequestDetailReview[];
  requestedReviewers: ChangeRequestDetailPerson[];
  checks: ChangeRequestDetailCheck[];
  comments: ChangeRequestDetailComment[];
  lastSyncedAt?: string;
};

export type ChangeRequestDetailActionRequest = {
  actionId: string;
  body?: string;
  threadId?: string;
  targetId?: string;
};

export type ChangeRequestDetailProps = {
  detail: ChangeRequestDetailModel | null;
  presentation?: "desktop" | "mobile";
  loading?: boolean;
  error?: string | null;
  onRefresh?: () => void;
  onRetry?: () => void;
  onAddContext?: (kind: "review" | "check" | "comment", content: string) => void;
  actions?: ChangeRequestDetailAction[];
  busyActionId?: string | null;
  onAction?: (request: ChangeRequestDetailActionRequest) => void | Promise<void>;
  headerActions?: ReactNode;
  notice?: ReactNode;
  contentLoading?: boolean;
};

function DetailState({ props }: { props: ChangeRequestDetailProps }) {
  if (props.loading)
    return (
      <div className="flex items-center justify-center py-8">
        <Spinner aria-label={t("integrations:loadingChangeRequest")} />
      </div>
    );
  if (props.error)
    return (
      <div
        role="alert"
        className="flex flex-col items-center gap-3 px-6 py-8 text-center text-sm text-muted-foreground"
      >
        <p>{props.error}</p>
        {props.onRetry ? (
          <Button type="button" variant="outline" className="min-h-11" onClick={props.onRetry}>
            {t("integrations:retry")}
          </Button>
        ) : null}
      </div>
    );
  return (
    <div className="flex items-center justify-center px-6 py-8 text-sm text-muted-foreground">
      {t("integrations:reviewUnavailable")}
    </div>
  );
}

export function ChangeRequestDetail(props: ChangeRequestDetailProps) {
  const { detail } = props;
  return (
    <div
      data-testid="change-request-detail"
      data-presentation={props.presentation ?? "desktop"}
      className="flex h-full min-h-0 flex-col"
    >
      {detail ? (
        <>
          <ChangeRequestDetailHeader props={props} detail={detail} />
          <Separator />
        </>
      ) : null}
      <ScrollArea
        data-testid="change-request-detail-scroll"
        className="min-h-0 flex-1 overflow-hidden"
      >
        <div
          data-testid="change-request-detail-content"
          className="box-border w-0 min-w-full max-w-full space-y-1 overflow-x-hidden p-3"
        >
          {!detail || props.contentLoading ? (
            <DetailState props={props} />
          ) : (
            <>
              {detail.description ? (
                <CollapsibleSection
                  title={t("integrations:description")}
                  count={1}
                  defaultOpen={false}
                >
                  <div className="px-2">
                    <PRMarkdownBody body={detail.description} />
                  </div>
                </CollapsibleSection>
              ) : null}
              <ChangeRequestReviews
                detail={detail}
                onAdd={props.onAddContext}
                onAction={props.onAction}
              />
              <ChangeRequestChecks detail={detail} onAdd={props.onAddContext} />
              <ChangeRequestComments key={detail.reviewKey} props={props} detail={detail} />
            </>
          )}
        </div>
      </ScrollArea>
      {detail?.lastSyncedAt ? (
        <>
          <Separator />
          <div className="px-3 py-2 text-center text-[10px] text-muted-foreground">
            {t("integrations:lastSynced", { relative: formatTimeAgo(detail.lastSyncedAt) })}
          </div>
        </>
      ) : null}
    </div>
  );
}
