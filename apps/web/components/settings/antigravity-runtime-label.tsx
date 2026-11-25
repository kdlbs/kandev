"use client";

import { useTranslation } from "react-i18next";
import type { AntigravityRuntimeVariant } from "@/lib/types/http";

export function AntigravityRuntimeLabel({
  variant,
  className,
}: {
  variant?: AntigravityRuntimeVariant;
  className?: string;
}) {
  const { t } = useTranslation();
  if (!variant) return null;
  return (
    <p className={className} data-testid="antigravity-runtime-label">
      {t(
        variant === "cli" ? "agents:antigravityRuntimeCLI" : "agents:antigravityRuntimeStandalone",
      )}
    </p>
  );
}
