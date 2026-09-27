import type { ExecutorProfile, ExecutorProvider } from "@/lib/types/http";

type Translate = (key: string, options?: Record<string, unknown>) => string;

const UNAVAILABLE_CAUSE_KEYS: Record<string, string> = {
  feature_disabled: "executors:providerFeatureDisabled",
  plugin_disabled: "executors:providerPluginDisabled",
  plugin_unavailable: "executors:providerPluginUnavailable",
};

export function executorProviderUnavailableReason(
  provider: ExecutorProvider | undefined,
  isProviderExecutor: boolean,
  t: Translate,
): string | null {
  if (!isProviderExecutor) return null;
  if (!provider) return t("executors:providerMissing");
  if (provider.available) return null;
  return t(
    UNAVAILABLE_CAUSE_KEYS[provider.availability_cause ?? ""] ?? "executors:providerUnavailable",
  );
}

export function localizedExecutorProviderMessage(
  provider: ExecutorProvider | undefined,
  reference: string,
  fallback: string,
  t: Translate,
): string {
  const messageKey = provider?.localized_messages?.[reference];
  if (!provider || !messageKey) return fallback;
  return t(`plugin-${provider.plugin_id}:${messageKey}`, { defaultValue: fallback });
}

export function executorProfileUnavailableReason(
  profile: ExecutorProfile,
  t: Translate,
): string | null {
  return executorProviderUnavailableReason(
    profile.provider,
    profile.executor_type === "plugin_remote",
    t,
  );
}

export function isExecutorProfileProviderAvailable(profile: ExecutorProfile | null): boolean {
  if (!profile || profile.executor_type !== "plugin_remote") return true;
  return profile.provider?.available === true;
}

function lifetimeDuration(seconds: number, t: Translate): string {
  const units = [
    { seconds: 86400, key: "Days" },
    { seconds: 3600, key: "Hours" },
    { seconds: 60, key: "Minutes" },
  ];
  const unit = units.find(
    (candidate) => seconds >= candidate.seconds && seconds % candidate.seconds === 0,
  );
  if (unit) return t(`executors:providerLifetime${unit.key}`, { count: seconds / unit.seconds });
  return t("executors:providerLifetimeSeconds", { count: seconds });
}

export function executorProviderRetentionText(
  provider: ExecutorProvider | undefined,
  t: Translate,
): string | null {
  if (!provider) return null;
  if (provider.capabilities.retention === "bounded") {
    const lifetime = provider.capabilities.maximum_lifetime_seconds;
    if (lifetime && lifetime > 0) {
      return t("executors:providerRetentionBounded", { duration: lifetimeDuration(lifetime, t) });
    }
  }
  const key = {
    ephemeral: "executors:providerRetentionEphemeral",
    persistent: "executors:providerRetentionPersistent",
    unknown: "executors:providerRetentionUnknown",
  }[provider.capabilities.retention];
  return t(key ?? "executors:providerRetentionUnknown");
}
