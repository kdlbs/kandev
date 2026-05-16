"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import type { CostBreakdownItem } from "@/lib/state/slices/office/types";
import { formatDollars } from "@/lib/utils";

type Props = {
  title: string;
  items: CostBreakdownItem[];
  labelPrefix: string;
};

export function CostBreakdownTable({ title, items, labelPrefix }: Props) {
  if (items.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{title}</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            No cost data yet. Costs are tracked when agents run tasks.
          </p>
        </CardContent>
      </Card>
    );
  }

  const maxSubcents = Math.max(...items.map((i) => i.total_subcents), 1);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="divide-y divide-border">
          <div className="flex items-center gap-4 py-2 text-xs text-muted-foreground font-medium">
            <span className="flex-1">{labelPrefix}</span>
            <span className="w-20 text-right">Events</span>
            <span className="w-24 text-right">Cost</span>
            <span className="w-32" />
          </div>
          {items.map((item) => {
            const pct = Math.round((item.total_subcents / maxSubcents) * 100);
            const label = item.group_label || item.group_key || "(unassigned)";
            const isFallbackToId = !item.group_label && Boolean(item.group_key);
            return (
              <div key={item.group_key} className="flex items-center gap-4 py-2 text-sm">
                <span className={`flex-1 truncate text-xs ${isFallbackToId ? "font-mono" : ""}`}>
                  {label}
                </span>
                <span className="w-20 text-right text-muted-foreground">{item.count}</span>
                <span className="w-24 text-right font-medium">
                  {formatDollars(item.total_subcents)}
                </span>
                <div className="w-32 h-2 bg-muted rounded-full overflow-hidden">
                  <div className="h-full rounded-full bg-blue-500" style={{ width: `${pct}%` }} />
                </div>
              </div>
            );
          })}
        </div>
      </CardContent>
    </Card>
  );
}
