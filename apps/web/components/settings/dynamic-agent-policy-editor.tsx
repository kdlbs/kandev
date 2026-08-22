"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  SettingsFieldDescription,
  SettingsFieldLabel,
  SettingsErrorText,
} from "@/components/settings/settings-typography";
import type { DynamicErrorClass, DynamicErrorPolicy } from "@/lib/types/agent-profile";

const MAX_RETRIES = 10;
const MAX_INITIAL_INTERVAL_SECONDS = 3600;
const MAX_WAIT_SECONDS = 7 * 24 * 60 * 60;

export function isDynamicErrorPolicyValid(policy: DynamicErrorPolicy): boolean {
  if (policy.retry.enabled) {
    if (policy.retry.maxRetries < 1 || policy.retry.maxRetries > MAX_RETRIES) return false;
    if (
      policy.retry.initialIntervalSeconds < 1 ||
      policy.retry.initialIntervalSeconds > MAX_INITIAL_INTERVAL_SECONDS
    ) {
      return false;
    }
  } else if (policy.retry.maxRetries !== 0 || policy.retry.initialIntervalSeconds !== 0) {
    return false;
  }
  if (policy.waitForReset.enabled) {
    return (
      policy.waitForReset.maxWaitSeconds >= 1 &&
      policy.waitForReset.maxWaitSeconds <= MAX_WAIT_SECONDS
    );
  }
  return policy.waitForReset.maxWaitSeconds === 0;
}

type PolicyTranslator = (key: string) => string;

function retryValidationMessage(
  policy: DynamicErrorPolicy,
  t: PolicyTranslator,
): string | undefined {
  if (!policy.retry.enabled) return undefined;
  if (policy.retry.maxRetries < 1 || policy.retry.maxRetries > MAX_RETRIES) {
    return t("agents:dynamicPolicyMaxRetriesValidation");
  }
  if (
    policy.retry.initialIntervalSeconds < 1 ||
    policy.retry.initialIntervalSeconds > MAX_INITIAL_INTERVAL_SECONDS
  ) {
    return t("agents:dynamicPolicyInitialIntervalValidation");
  }
  return undefined;
}

function waitValidationMessage(
  policy: DynamicErrorPolicy,
  t: PolicyTranslator,
): string | undefined {
  if (
    policy.waitForReset.enabled &&
    (policy.waitForReset.maxWaitSeconds < 1 ||
      policy.waitForReset.maxWaitSeconds > MAX_WAIT_SECONDS)
  ) {
    return t("agents:dynamicPolicyMaxWaitValidation");
  }
  return undefined;
}

function DynamicPolicyOptionHelp({ option }: { option: "retry" | "wait" | "outcome" }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const titleKey = `agents:dynamicPolicy${option[0].toUpperCase()}${option.slice(1)}`;
  const helpKey = `${titleKey}Help`;
  const optionLabel = t(titleKey);

  return (
    <Tooltip open={open} onOpenChange={setOpen}>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="size-11 shrink-0 cursor-help text-muted-foreground sm:size-7"
          aria-label={t("agents:dynamicPolicyOptionInfo", { option: optionLabel })}
          onClick={() => setOpen((current) => !current)}
          data-testid={`dynamic-policy-option-help-${option}`}
        >
          <IconInfoCircle className="size-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs text-xs leading-relaxed">{t(helpKey)}</TooltipContent>
    </Tooltip>
  );
}

// eslint-disable-next-line max-lines-per-function -- keeps the two-class policy form cohesive.
export function DynamicPolicyEditor({
  errorClass,
  policy,
  onChange,
}: {
  errorClass: DynamicErrorClass;
  policy: DynamicErrorPolicy;
  onChange: (patch: Partial<DynamicErrorPolicy>) => void;
}) {
  const { t } = useTranslation();
  const isTransient = errorClass === "transient";
  const title = t(isTransient ? "agents:dynamicTransientErrors" : "agents:dynamicHardErrors");
  const description = t(
    isTransient
      ? "agents:dynamicTransientErrorsDescription"
      : "agents:dynamicHardErrorsDescription",
  );
  const retryLabel = t("agents:dynamicPolicyRetry");
  const waitLabel = t("agents:dynamicPolicyWaitForReset");
  const outcomeLabel = t("agents:dynamicPolicyAfterExhausted");
  const retryError = retryValidationMessage(policy, t);
  const waitError = waitValidationMessage(policy, t);
  const retryMaxInvalid = retryError === t("agents:dynamicPolicyMaxRetriesValidation");
  const retryIntervalInvalid = retryError === t("agents:dynamicPolicyInitialIntervalValidation");
  const updateRetry = (patch: Partial<DynamicErrorPolicy["retry"]>) =>
    onChange({ retry: { ...policy.retry, ...patch } });
  const updateWait = (patch: Partial<DynamicErrorPolicy["waitForReset"]>) =>
    onChange({ waitForReset: { ...policy.waitForReset, ...patch } });
  const schedule = policy.retry.enabled
    ? t("agents:dynamicPolicySchedule", {
        first: policy.retry.initialIntervalSeconds,
        retries: policy.retry.maxRetries,
      })
    : t("agents:dynamicPolicyNoRetry");

  return (
    <section
      className="space-y-4 rounded-md border bg-muted/10 p-4"
      data-testid={`dynamic-policy-${errorClass}`}
    >
      <div className="space-y-1">
        <h4 className="text-sm font-semibold">{title}</h4>
        <p className="text-xs leading-relaxed text-muted-foreground">{description}</p>
      </div>
      <div className="space-y-3">
        <div className="flex min-h-11 items-center justify-between gap-3 rounded-md border p-3">
          <div className="min-w-0">
            <SettingsFieldLabel className="flex items-center gap-1.5">
              {retryLabel}
              <DynamicPolicyOptionHelp option="retry" />
            </SettingsFieldLabel>
            <SettingsFieldDescription>{schedule}</SettingsFieldDescription>
          </div>
          <Switch
            checked={policy.retry.enabled}
            onCheckedChange={(enabled) =>
              updateRetry(
                enabled
                  ? {
                      enabled,
                      maxRetries: policy.retry.maxRetries || 1,
                      initialIntervalSeconds: policy.retry.initialIntervalSeconds || 5,
                    }
                  : { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
              )
            }
            aria-label={retryLabel}
          />
        </div>
        {policy.retry.enabled && (
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="space-y-1.5">
              <span className="text-xs font-medium">{t("agents:dynamicPolicyMaxRetries")}</span>
              <Input
                type="number"
                min={1}
                max={10}
                value={policy.retry.maxRetries}
                onChange={(event) => updateRetry({ maxRetries: Number(event.target.value) || 0 })}
                className="min-h-11"
                aria-label={t("agents:dynamicPolicyMaxRetries")}
                aria-invalid={retryMaxInvalid}
              />
              {retryMaxInvalid && <SettingsErrorText>{retryError}</SettingsErrorText>}
            </label>
            <label className="space-y-1.5">
              <span className="text-xs font-medium">
                {t("agents:dynamicPolicyInitialInterval")}
              </span>
              <Input
                type="number"
                min={1}
                max={3600}
                value={policy.retry.initialIntervalSeconds}
                onChange={(event) =>
                  updateRetry({ initialIntervalSeconds: Number(event.target.value) || 0 })
                }
                className="min-h-11"
                aria-label={t("agents:dynamicPolicyInitialInterval")}
                aria-invalid={retryIntervalInvalid}
              />
              {retryIntervalInvalid && <SettingsErrorText>{retryError}</SettingsErrorText>}
            </label>
          </div>
        )}
        <div className="flex min-h-11 items-center justify-between gap-3 rounded-md border p-3">
          <div className="min-w-0">
            <SettingsFieldLabel className="flex items-center gap-1.5">
              {waitLabel}
              <DynamicPolicyOptionHelp option="wait" />
            </SettingsFieldLabel>
            <SettingsFieldDescription>
              {t("agents:dynamicPolicyWaitDescription")}
            </SettingsFieldDescription>
          </div>
          <Switch
            checked={policy.waitForReset.enabled}
            onCheckedChange={(enabled) =>
              updateWait(
                enabled
                  ? { enabled, maxWaitSeconds: policy.waitForReset.maxWaitSeconds || 300 }
                  : { enabled: false, maxWaitSeconds: 0 },
              )
            }
            aria-label={waitLabel}
          />
        </div>
        {policy.waitForReset.enabled && (
          <label className="block space-y-1.5">
            <span className="text-xs font-medium">{t("agents:dynamicPolicyMaxWait")}</span>
            <Input
              type="number"
              min={1}
              max={604800}
              value={policy.waitForReset.maxWaitSeconds}
              onChange={(event) => updateWait({ maxWaitSeconds: Number(event.target.value) || 0 })}
              className="min-h-11"
              aria-label={t("agents:dynamicPolicyMaxWait")}
              aria-invalid={Boolean(waitError)}
            />
            {waitError && <SettingsErrorText>{waitError}</SettingsErrorText>}
          </label>
        )}
        <div className="space-y-1.5">
          <SettingsFieldLabel className="flex items-center gap-1.5">
            {outcomeLabel}
            <DynamicPolicyOptionHelp option="outcome" />
          </SettingsFieldLabel>
          <Select
            value={policy.onExhausted}
            onValueChange={(value) => onChange({ onExhausted: value as "skip" | "stop" })}
          >
            <SelectTrigger className="min-h-11 w-full" aria-label={outcomeLabel}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="skip">{t("agents:dynamicPolicySkip")}</SelectItem>
              <SelectItem value="stop">{t("agents:dynamicPolicyStop")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
    </section>
  );
}
