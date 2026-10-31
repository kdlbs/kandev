"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconCheck, IconAlertCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useAppStore } from "@/components/state-provider";
import type { MCPRegistrationReceipt } from "@/hooks/domains/workspace/use-mcp-registration-completion";

export function MCPRegistrationCompletion({
  receipt,
  onDone,
  onUse,
}: {
  receipt: MCPRegistrationReceipt;
  onDone: () => void;
  onUse: () => void;
}) {
  const { t } = useTranslation();
  const workspace = useAppStore((state) =>
    state.workspaces.items.find((item) => item.id === receipt.workspaceId),
  );
  const titleRef = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    titleRef.current?.focus();
  }, [receipt.definitionId]);
  return (
    <section
      className="min-w-0 space-y-3 rounded-lg border bg-card p-4"
      aria-labelledby="mcp-registration-title"
      data-testid="mcp-registration-completion"
    >
      <div className="flex min-w-0 items-start gap-3">
        <IconCheck className="mt-0.5 size-5 shrink-0 text-primary" aria-hidden="true" />
        <div className="min-w-0 space-y-1">
          <h2
            ref={titleRef}
            tabIndex={-1}
            id="mcp-registration-title"
            className="break-words text-base font-semibold outline-none"
          >
            {t("settings:mcpRegistrationSavedTo", {
              workspace: workspace?.name ?? receipt.workspaceId,
            })}
          </h2>
          <p className="break-words text-sm font-medium">{receipt.displayName}</p>
          <p className="text-sm text-muted-foreground">{t("settings:mcpRegistrationSavedOnly")}</p>
        </div>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          className={controlSizingClassName("standard", "w-full cursor-pointer sm:w-auto")}
          onClick={onUse}
        >
          {t("settings:mcpUseIn")}
        </Button>
        <Button
          type="button"
          variant="outline"
          className={controlSizingClassName("standard", "w-full cursor-pointer sm:w-auto")}
          onClick={onDone}
        >
          {t("settings:done")}
        </Button>
      </div>
    </section>
  );
}

export function MCPRegistrationUncertain({ onReview }: { onReview: () => Promise<void> }) {
  const { t } = useTranslation();
  const [reviewing, setReviewing] = useState(false);
  return (
    <section className="space-y-3 rounded-lg border p-4" data-testid="mcp-registration-uncertain">
      <p role="alert" className="flex items-start gap-2 text-sm">
        <IconAlertCircle className="size-5 shrink-0" aria-hidden="true" />
        {t("settings:mcpRegistrationUncertain")}
      </p>
      <Button
        type="button"
        variant="outline"
        disabled={reviewing}
        className={controlSizingClassName("standard", "w-full cursor-pointer sm:w-auto")}
        onClick={async () => {
          setReviewing(true);
          try {
            await onReview();
          } finally {
            setReviewing(false);
          }
        }}
      >
        {reviewing ? t("common:loading") : t("settings:mcpReviewSavedServers")}
      </Button>
    </section>
  );
}
