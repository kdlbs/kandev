"use client";

import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";

export function SessionErrorDetails({
  children,
  label,
  testId,
  textTestId,
}: {
  children: string;
  label?: string;
  testId?: string;
  textTestId?: string;
}) {
  const { t } = useTranslation();
  const [copyStatus, setCopyStatus] = useState<"copied" | "failed" | null>(null);
  const sanitized = sanitizeSessionErrorDetails(children, 4097);
  const text =
    sanitized.length > 4096
      ? `${sanitized.slice(0, 4096).replace(/[\uD800-\uDBFF]$/u, "")}\n${t("task:outputTruncated")}`
      : sanitized;
  if (!text) return null;
  const copy = async () => {
    setCopyStatus((await copyToClipboard(text)) ? "copied" : "failed");
  };
  return (
    <details className="mt-2 w-full min-w-0 text-xs text-muted-foreground" data-testid={testId}>
      <summary
        data-testid={testId ? `${testId}-summary` : undefined}
        className="flex min-h-7 cursor-pointer list-none items-center gap-1.5 rounded-sm focus-visible:ring-2 focus-visible:ring-ring max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      >
        <IconChevronDown className="size-3.5 shrink-0" aria-hidden="true" />
        {label ?? t("chat:technicalDetails")}
      </summary>
      <pre
        className="w-full min-w-0 whitespace-pre-wrap wrap-anywhere rounded bg-muted/50 p-2 font-mono text-[11px] leading-relaxed"
        data-testid={textTestId}
      >
        {text}
      </pre>
      <div className="mt-2 flex min-w-0 flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={() => void copy()}
          className={controlSizingClassName("standard", "w-full cursor-pointer md:w-auto")}
        >
          {t("task:copyRecoveryDetails")}
        </Button>
        <span role="status" aria-live="polite">
          {copyStatus === "copied" ? t("task:recoveryDetailsCopied") : null}
          {copyStatus === "failed" ? t("task:recoveryDetailsCopyFailed") : null}
        </span>
      </div>
    </details>
  );
}
