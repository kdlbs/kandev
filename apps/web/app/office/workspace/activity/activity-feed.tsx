"use client";

import { useEffect, useState } from "react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { toast } from "@/lib/toast/sonner";
import { listActivity } from "@/lib/api/domains/office-api";
import type { ActivityEntry } from "@/lib/state/slices/office/types";
import { ActivityRow } from "./activity-row";
import { EmptyState } from "../../components/shared/empty-state";
import { PageHeader } from "../../components/shared/page-header";
import { useTranslation } from "react-i18next";
// Module-level `t` for the error-only string inside the fetching effect below:
// putting the hook's `t` in that dep array would re-issue the request on every
// locale switch.
import { t as staticT } from "@/lib/i18n";

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale.
// The `value`s are the wire filter ids sent to `listActivity`.
const FILTER_OPTIONS = [
  { value: "all", labelKey: "office:activityFilterAll" },
  { value: "agent", labelKey: "office:agent" },
  { value: "task", labelKey: "office:activityFilterTask" },
  { value: "project", labelKey: "office:project" },
  { value: "budget", labelKey: "office:activityFilterBudget" },
  { value: "approval", labelKey: "office:approval" },
  { value: "system", labelKey: "office:system" },
];

export function ActivityFeed({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const [entries, setEntries] = useState<ActivityEntry[]>([]);
  const [filterType, setFilterType] = useState("all");

  useEffect(() => {
    listActivity(workspaceId, filterType)
      .then((res) => setEntries(res.activity ?? []))
      .catch((err) => {
        toast.error(err instanceof Error ? err.message : staticT("office:failedToLoadActivity"));
      });
  }, [workspaceId, filterType]);

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office:activity")}
        action={
          <Select value={filterType} onValueChange={setFilterType}>
            <SelectTrigger className="w-[140px] h-8 text-xs cursor-pointer">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {FILTER_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value} className="cursor-pointer">
                  {t(opt.labelKey)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />

      {entries.length === 0 ? (
        <EmptyState
          message={t("office:noActivityYet")}
          description={t("office:actionsByAgentsAndUsersAre")}
        />
      ) : (
        <div className="border border-border rounded-lg divide-y divide-border">
          {entries.map((entry) => (
            <ActivityRow key={entry.id} entry={entry} />
          ))}
        </div>
      )}
    </div>
  );
}
