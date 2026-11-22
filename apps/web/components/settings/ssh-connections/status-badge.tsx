"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { cn } from "@/lib/utils";
import type { ConnectionState } from "./connection-presentation";

export function ConnectionStatusBadge({
  state,
  className,
}: {
  state: ConnectionState;
  className?: string;
}) {
  const { t } = useTranslation();
  return (
    <Badge
      variant="outline"
      className={cn(
        "font-normal shrink-0",
        state === "ready" &&
          "border-success/35 bg-success/10 text-green-800 dark:bg-success/10 dark:text-green-300",
        className,
      )}
    >
      {t(`settings:sshManager.ui.states.${state}`)}
    </Badge>
  );
}
