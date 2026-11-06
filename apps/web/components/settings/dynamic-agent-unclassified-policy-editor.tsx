"use client";

import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import {
  SettingsFieldDescription,
  SettingsFieldLabel,
  SettingsErrorText,
} from "@/components/settings/settings-typography";
import { DynamicPolicyOptionHelp } from "@/components/settings/dynamic-agent-policy-editor";
import type { DynamicUnclassifiedPolicy } from "@/lib/types/agent-profile";
import { settingsControlClassName } from "./settings-control";

export const DYNAMIC_UNCLASSIFIED_MIN_THRESHOLD = 2;
export const DYNAMIC_UNCLASSIFIED_MAX_THRESHOLD = 10;

export function isDynamicUnclassifiedPolicyValid(policy: DynamicUnclassifiedPolicy): boolean {
  if (!policy.enabled) return policy.consecutiveFailureThreshold === 0;
  return (
    Number.isInteger(policy.consecutiveFailureThreshold) &&
    policy.consecutiveFailureThreshold >= DYNAMIC_UNCLASSIFIED_MIN_THRESHOLD &&
    policy.consecutiveFailureThreshold <= DYNAMIC_UNCLASSIFIED_MAX_THRESHOLD
  );
}

export function DynamicUnclassifiedPolicySummary({
  policy,
}: {
  policy: DynamicUnclassifiedPolicy;
}) {
  const { t } = useTranslation();
  return (
    <span className="block text-xs">
      {policy.enabled
        ? t("agents:dynamicUnclassifiedSummaryOn", {
            threshold: policy.consecutiveFailureThreshold,
          })
        : t("agents:dynamicUnclassifiedSummaryOff")}
    </span>
  );
}

export function DynamicUnclassifiedPolicyEditor({
  policy,
  onChange,
}: {
  policy: DynamicUnclassifiedPolicy;
  onChange: (patch: Partial<DynamicUnclassifiedPolicy>) => void;
}) {
  const { t } = useTranslation();
  const label = t("agents:dynamicPolicyUnclassified");
  const thresholdInvalid = policy.enabled && !isDynamicUnclassifiedPolicyValid(policy);

  return (
    <section
      className="space-y-3 rounded-md border bg-muted/10 p-4"
      data-testid="dynamic-policy-unclassified"
    >
      <div className="space-y-1">
        <h4 className="text-sm font-semibold">{t("agents:dynamicUnclassifiedErrors")}</h4>
        <p className="text-xs leading-relaxed text-muted-foreground">
          {t("agents:dynamicUnclassifiedDescription")}
        </p>
      </div>
      <div className="space-y-3">
        <div className="flex min-h-11 items-center justify-between gap-3 rounded-md border p-3">
          <div className="min-w-0">
            <SettingsFieldLabel className="flex items-center gap-1.5">
              {label}
              <DynamicPolicyOptionHelp option="unclassified" />
            </SettingsFieldLabel>
            <SettingsFieldDescription>
              {t("agents:dynamicUnclassifiedSwitchDescription")}
            </SettingsFieldDescription>
          </div>
          <Switch
            checked={policy.enabled}
            onCheckedChange={(enabled) =>
              onChange(
                enabled
                  ? {
                      enabled,
                      consecutiveFailureThreshold:
                        policy.consecutiveFailureThreshold || DYNAMIC_UNCLASSIFIED_MIN_THRESHOLD,
                    }
                  : { enabled: false, consecutiveFailureThreshold: 0 },
              )
            }
            aria-label={label}
          />
        </div>
        {policy.enabled && (
          <label className="block space-y-1.5">
            <span className="text-xs font-medium">{t("agents:dynamicUnclassifiedThreshold")}</span>
            <Input
              type="number"
              min={DYNAMIC_UNCLASSIFIED_MIN_THRESHOLD}
              max={DYNAMIC_UNCLASSIFIED_MAX_THRESHOLD}
              step={1}
              value={policy.consecutiveFailureThreshold}
              onChange={(event) =>
                onChange({ consecutiveFailureThreshold: Number(event.target.value) || 0 })
              }
              className={settingsControlClassName()}
              aria-label={t("agents:dynamicUnclassifiedThreshold")}
              aria-invalid={thresholdInvalid}
            />
            {thresholdInvalid && (
              <SettingsErrorText>
                {t("agents:dynamicUnclassifiedThresholdValidation")}
              </SettingsErrorText>
            )}
          </label>
        )}
      </div>
    </section>
  );
}
