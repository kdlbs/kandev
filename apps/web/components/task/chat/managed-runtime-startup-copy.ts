import type { useTranslation } from "react-i18next";

export type ManagedRuntimeStartupMetadata = {
  startup_reason?: string;
  startup_attempts?: number;
  startup_npm_code?: string;
};

type Translate = ReturnType<typeof useTranslation>["t"];

export function managedRuntimeStartupCopy(
  metadata: ManagedRuntimeStartupMetadata | undefined,
  t: Translate,
) {
  const attempts = Number.isInteger(metadata?.startup_attempts)
    ? Math.max(0, metadata?.startup_attempts ?? 0)
    : 0;
  // i18n-exempt: these are stable backend reason codes, not user-facing copy.
  switch (metadata?.startup_reason) {
    case "npm_transient":
      if (attempts > 1)
        return {
          title: t("chat:managedRuntimeNpmTitle"),
          summary: t("chat:managedRuntimeStartupTransientBody", { attempts }),
        };
      break;
    case "early_exit":
      if (attempts > 1)
        return {
          title: t("chat:managedRuntimeStartupEarlyExitTitle"),
          summary: t("chat:managedRuntimeStartupEarlyExitBody", { attempts }),
        };
      break;
    case "cleanup_failed":
      return {
        title: t("chat:managedRuntimeStartupCleanupTitle"),
        summary: t("chat:managedRuntimeStartupCleanupBody"),
      };
    case "permanent_npm_error":
      return {
        title: t("chat:managedRuntimeNpmTitle"),
        summary: t("chat:managedRuntimeStartupPermanentBody"),
      };
  }

  return {
    title: t("chat:managedRuntimeStartupTitle"),
    summary:
      attempts > 1
        ? t("chat:managedRuntimeStartupRetryBody", { attempts })
        : t("chat:managedRuntimeStartupFirstAttemptBody"),
  };
}
