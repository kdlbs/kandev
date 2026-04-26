"use client";

import type { ComponentType } from "react";
import { Card } from "@kandev/ui/card";

type Props = {
  icon: ComponentType<{ className?: string }>;
  value: string | number;
  label: string;
  description?: string;
};

export function MetricCard({ icon: Icon, value, label, description }: Props) {
  return (
    <Card className="p-4">
      <div className="flex justify-between items-start">
        <div>
          <p className="text-2xl sm:text-3xl font-bold">{value}</p>
          <p className="text-xs sm:text-sm text-muted-foreground mt-1">{label}</p>
          {description && <p className="text-xs text-muted-foreground">{description}</p>}
        </div>
        <Icon className="h-5 w-5 text-muted-foreground" />
      </div>
    </Card>
  );
}
