"use client";

import { useTranslation } from "react-i18next";
import type { CommandSummary } from "@/lib/types/execution";

export function CommandDetails({ command }: { command: CommandSummary }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1 break-words">
      <code>{command.executable}</code>
      {command.omitted_arguments > 0 && (
        <p className="text-muted-foreground">
          {t("agents:executionHiddenArguments", { count: command.omitted_arguments })}
        </p>
      )}
      {command.references.map((reference) => (
        <code className="block break-all" key={reference}>
          {reference}
        </code>
      ))}
    </div>
  );
}
