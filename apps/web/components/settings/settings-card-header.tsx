import type { ReactNode } from "react";
import { CardHeader } from "@kandev/ui/card";

import { cn } from "@/lib/utils";
import { SETTINGS_TYPOGRAPHY } from "./settings-typography";

type SettingsCardHeaderProps = {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  className?: string;
  titleTestId?: string;
};

/** Shared settings card heading with a readable mobile action layout. */
export function SettingsCardHeader({
  title,
  description,
  actions,
  className,
  titleTestId,
}: SettingsCardHeaderProps) {
  return (
    <CardHeader
      className={cn("flex flex-col gap-3 md:flex-row md:items-start md:justify-between", className)}
    >
      <div className="min-w-0 space-y-1">
        <h3 className={SETTINGS_TYPOGRAPHY.cardTitle} data-testid={titleTestId}>
          {title}
        </h3>
        {description && <p className={SETTINGS_TYPOGRAPHY.cardDescription}>{description}</p>}
      </div>
      {actions && <div className="w-full shrink-0 md:w-auto">{actions}</div>}
    </CardHeader>
  );
}
