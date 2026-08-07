"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { IconExternalLink } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { normalizeRemediationUrl } from "@/lib/remediation-url";

/**
 * Shared localized external link for the allowlisted provider remediation
 * destination. Defense-in-depth: the URL is re-validated at the browser edge,
 * so an invalid or hostile metadata value renders nothing. The 44px touch
 * target on phones matches the recovery buttons beside it.
 */
export function RemediationLink({ url, className }: { url?: string; className?: string }) {
  const { t } = useTranslation();
  const safe = useMemo(() => normalizeRemediationUrl(url), [url]);
  if (!safe) return null;
  return (
    <a
      href={safe}
      target="_blank"
      rel="noopener noreferrer"
      data-testid="remediation-link"
      className={cn(
        "inline-flex min-h-11 items-center gap-1.5 text-xs text-primary underline underline-offset-4 cursor-pointer sm:min-h-8",
        className,
      )}
    >
      <IconExternalLink className="h-3 w-3 flex-shrink-0" aria-hidden="true" />
      {t("chat:openProviderPage")}
    </a>
  );
}
