"use client";

import { useTranslation } from "react-i18next";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import type { ReactNode } from "react";
import { Badge } from "@kandev/ui/badge";
import type { InboxTab } from "@/lib/failed-inbox/inbox-tab";

// Absent for a not-yet-known or zero count; a truncated count carries a "+"
// suffix (AC-UI-INBOX-FAILED-001.16, .17) -- the same capped presentation the
// sidebar badge already uses.
function badgeText(count: number | undefined, truncated: boolean): string | null {
  if (typeof count !== "number" || count <= 0) return null;
  return truncated ? `${count}+` : `${count}`;
}

export function InboxTabStrip({
  selectedTab,
  onSelectTab,
  needsYouCount,
  needsYouHasMore,
  failedCount,
  failedTruncated,
  historyCount,
  children,
}: {
  selectedTab: InboxTab;
  onSelectTab: (tab: InboxTab) => void;
  needsYouCount: number;
  needsYouHasMore: boolean;
  failedCount: number | undefined;
  failedTruncated: boolean;
  historyCount: number;
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  const needsYouBadge = badgeText(needsYouCount, needsYouHasMore);
  const failedBadge = badgeText(failedCount, failedTruncated);
  const historyBadge = badgeText(historyCount, false);

  return (
    <Tabs value={selectedTab} onValueChange={(value) => onSelectTab(value as InboxTab)}>
      <TabsList variant="line" className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
        <TabsTrigger
          value="needs-you"
          className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        >
          {t("needsYouInbox:tabLabel")}
          {needsYouBadge && (
            <Badge variant="secondary" data-testid="inbox-tab-needs-you-badge">
              {needsYouBadge}
            </Badge>
          )}
        </TabsTrigger>
        <TabsTrigger value="failed" className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
          {t("failedInbox:tabLabel")}
          {failedBadge && (
            <Badge variant="secondary" data-testid="inbox-tab-failed-badge">
              {failedBadge}
            </Badge>
          )}
        </TabsTrigger>
        <TabsTrigger
          value="history"
          data-testid="inbox-tab-history"
          className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        >
          {t("inboxHistory:tabLabel")}
          {historyBadge && (
            <Badge variant="secondary" data-testid="inbox-tab-history-badge">
              {historyBadge}
            </Badge>
          )}
        </TabsTrigger>
      </TabsList>
      {children}
    </Tabs>
  );
}
