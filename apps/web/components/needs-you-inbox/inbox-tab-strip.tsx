"use client";

import { useTranslation } from "react-i18next";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
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
}: {
  selectedTab: InboxTab;
  onSelectTab: (tab: InboxTab) => void;
  needsYouCount: number;
  needsYouHasMore: boolean;
  failedCount: number | undefined;
  failedTruncated: boolean;
}) {
  const { t } = useTranslation();
  const needsYouBadge = badgeText(needsYouCount, needsYouHasMore);
  const failedBadge = badgeText(failedCount, failedTruncated);

  return (
    <Tabs value={selectedTab} onValueChange={(value) => onSelectTab(value as InboxTab)}>
      <TabsList variant="line">
        <TabsTrigger value="needs-you">
          {t("needsYouInbox:tabLabel")}
          {needsYouBadge && (
            <Badge variant="secondary" data-testid="inbox-tab-needs-you-badge">
              {needsYouBadge}
            </Badge>
          )}
        </TabsTrigger>
        <TabsTrigger value="failed">
          {t("failedInbox:tabLabel")}
          {failedBadge && (
            <Badge variant="secondary" data-testid="inbox-tab-failed-badge">
              {failedBadge}
            </Badge>
          )}
        </TabsTrigger>
      </TabsList>
    </Tabs>
  );
}
