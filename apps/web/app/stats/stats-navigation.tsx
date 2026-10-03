"use client";

import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import { useTranslation } from "react-i18next";
import { usePathname, useRouter, useSearchParams } from "@/lib/routing/client-router";
import { RANGE_KEYS, type RangeKey } from "./stats-utils";

export const RANGE_LABEL_KEYS: Record<RangeKey, string> = {
  week: "stats:rangeLastWeek",
  month: "stats:rangeLastMonth",
  all: "stats:rangeAllTime",
};

type StatsNavigationProps = {
  className?: string;
};

export function StatsNavigation({ className }: StatsNavigationProps) {
  const { t } = useTranslation();
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const activeSection = pathname === "/stats/token-usage" ? "token-usage" : "overview";

  const navigate = (section: string) => {
    const href = section === "token-usage" ? "/stats/token-usage" : "/stats";
    const query = searchParams.toString();
    router.push(query ? `${href}?${query}` : href, { scroll: false });
  };

  return (
    <Tabs
      value={activeSection}
      onValueChange={navigate}
      className={className}
      aria-label={t("stats:statisticsSections")}
    >
      <TabsList className="h-11 shrink-0 md:h-7" variant="line">
        <TabsTrigger value="overview" className="h-11 cursor-pointer px-3 text-xs md:h-6 md:px-2">
          {t("stats:overview")}
        </TabsTrigger>
        <TabsTrigger
          value="token-usage"
          className="h-11 cursor-pointer px-3 text-xs md:h-6 md:px-2"
        >
          {t("stats:tokenUsage")}
        </TabsTrigger>
      </TabsList>
    </Tabs>
  );
}

type StatsRangeToggleProps = {
  range: RangeKey;
  onChange: (range: RangeKey) => void;
};

export function StatsRangeToggle({ range, onChange }: StatsRangeToggleProps) {
  const { t } = useTranslation();
  return (
    <ToggleGroup
      type="single"
      value={range}
      onValueChange={(value) => {
        if (value) onChange(value as RangeKey);
      }}
      variant="outline"
      className="h-11 shrink-0 md:h-7"
      aria-label={t("stats:dateRange")}
    >
      {RANGE_KEYS.map((key) => (
        <ToggleGroupItem
          key={key}
          value={key}
          className="h-11 cursor-pointer px-3 text-xs data-[state=on]:bg-muted data-[state=on]:text-foreground md:h-7 md:px-2"
        >
          {t(RANGE_LABEL_KEYS[key])}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
