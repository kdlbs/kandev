import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { RetryPolicy } from "@/lib/types/automation";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconInfoCircle } from "@tabler/icons-react";
import { SegmentedChoice, type SegmentedOption } from "./segmented-choice";
import { retryPolicyAfterModeChange, type FormState } from "./automation-payload";
type Props = {
  policy: RetryPolicy;
  savedPolicy: RetryPolicy;
  updateField: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
};

function RetryModeSelector({ policy, updateField }: Pick<Props, "policy" | "updateField">) {
  const { t } = useTranslation();
  const options: SegmentedOption<RetryPolicy["mode"]>[] = [
    {
      value: "disabled",
      label: t("automations:retryDisabledTitle"),
      description: t("automations:retryDisabledDescription"),
    },
    {
      value: "finite",
      label: t("automations:retryFiniteTitle"),
      description: t("automations:retryFiniteDescription"),
    },
    {
      value: "infinite",
      label: t("automations:retryInfiniteTitle"),
      description: t("automations:retryInfiniteDescription"),
    },
  ];
  return (
    <SegmentedChoice
      id="automation-retry"
      label={t("automations:retryTitle")}
      value={policy.mode}
      options={options}
      onValueChange={(mode) => updateField("retryPolicy", retryPolicyAfterModeChange(policy, mode))}
      testId="automation-retry-modes"
    />
  );
}

function RetryTuningControls({
  policy,
  updatePolicy,
}: {
  policy: RetryPolicy;
  updatePolicy: (patch: Partial<RetryPolicy>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <div className="space-y-1.5">
        <Label htmlFor="automation-retry-max">{t("automations:retryMaxLabel")}</Label>
        <Input
          id="automation-retry-max"
          inputMode="numeric"
          value={policy.max_retries}
          disabled={policy.mode === "infinite"}
          onChange={(event) => updatePolicy({ max_retries: event.target.value })}
          aria-describedby="automation-retry-max-help"
        />
        <p id="automation-retry-max-help" className="text-xs text-muted-foreground">
          {t("automations:retryMaxHelp")}
        </p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="automation-retry-delay">{t("automations:retryDelayLabel")}</Label>
        <Input
          id="automation-retry-delay"
          inputMode="numeric"
          value={policy.delay_seconds}
          onChange={(event) => updatePolicy({ delay_seconds: event.target.value })}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="automation-retry-backoff">{t("automations:retryBackoffLabel")}</Label>
        <Select
          value={policy.backoff}
          onValueChange={(value) => updatePolicy({ backoff: value as RetryPolicy["backoff"] })}
        >
          <SelectTrigger id="automation-retry-backoff" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="fixed">{t("automations:retryBackoffFixed")}</SelectItem>
            <SelectItem value="exponential">{t("automations:retryBackoffExponential")}</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}

function RetryHistorySelector({
  policy,
  updatePolicy,
}: {
  policy: RetryPolicy;
  updatePolicy: (patch: Partial<RetryPolicy>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <span className="flex items-center gap-1.5">
        <Label htmlFor="automation-retry-history">{t("automations:retryHistoryLabel")}</Label>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              className="inline-flex cursor-help items-center text-muted-foreground max-md:min-h-11 max-md:min-w-11 max-md:justify-center"
              aria-label={t("automations:retryHistoryHelp")}
            >
              <IconInfoCircle aria-hidden className="h-3.5 w-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent className="max-w-[280px]">
            {t("automations:retryHistoryHelp")}
          </TooltipContent>
        </Tooltip>
      </span>
      <Select
        value={policy.history_mode}
        onValueChange={(value) =>
          updatePolicy({ history_mode: value as RetryPolicy["history_mode"] })
        }
      >
        <SelectTrigger id="automation-retry-history" className="w-full sm:w-48">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="attempts">{t("automations:retryHistoryAttempts")}</SelectItem>
          <SelectItem value="timeline">{t("automations:retryHistoryTimeline")}</SelectItem>
        </SelectContent>
      </Select>
    </div>
  );
}

export function RetryPolicySection({ policy, savedPolicy, updateField }: Props) {
  const dirty = JSON.stringify(policy) !== JSON.stringify(savedPolicy);
  const updatePolicy = (patch: Partial<RetryPolicy>) =>
    updateField("retryPolicy", { ...policy, ...patch });
  return (
    <div className="space-y-3" data-settings-dirty={dirty}>
      <RetryModeSelector policy={policy} updateField={updateField} />
      {policy.mode !== "disabled" && (
        <>
          <RetryTuningControls policy={policy} updatePolicy={updatePolicy} />
          <RetryHistorySelector policy={policy} updatePolicy={updatePolicy} />
        </>
      )}
    </div>
  );
}
