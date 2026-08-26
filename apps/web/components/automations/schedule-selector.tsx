"use client";

import { useTranslation } from "react-i18next";
import { useEffect, useMemo, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from "@kandev/ui/command";
import { Input } from "@kandev/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconInfoCircle, IconSelector } from "@tabler/icons-react";

import {
  browserTimeZone,
  buildExpression,
  formatInZone,
  nextRun,
  parseExpression,
  timeZoneAbbreviation,
  timeZoneOptions,
  usesTimezone,
  type Frequency,
  type ScheduleSpec,
} from "./schedule-expression";

type ScheduleSelectorProps = {
  config: Record<string, unknown> | null;
  isDirty?: boolean;
  onChange: (config: Record<string, unknown>) => void;
};

type ScheduleFrequency = Frequency | "none";

// Catalog keys rather than copy: both tables are built at module load, so a
// `t()` here would freeze at the boot locale (docs/i18n.md). Each render
// resolves them.
const FREQUENCY_OPTIONS: { value: ScheduleFrequency; labelKey: string }[] = [
  { value: "none", labelKey: "automations:scheduleNone" },
  { value: "every-5m", labelKey: "automations:frequencyEvery5Minutes" },
  { value: "every-15m", labelKey: "automations:frequencyEvery15Minutes" },
  { value: "every-30m", labelKey: "automations:frequencyEvery30Minutes" },
  { value: "hourly", labelKey: "automations:frequencyEveryHour" },
  { value: "every-6h", labelKey: "automations:frequencyEvery6Hours" },
  { value: "daily", labelKey: "automations:frequencyEveryDay" },
  { value: "weekly", labelKey: "automations:frequencyEveryWeek" },
  { value: "custom", labelKey: "automations:frequencyCustom" },
];

const WEEKDAY_KEYS = [
  "common:weekdaySunday",
  "common:weekdayMonday",
  "common:weekdayTuesday",
  "common:weekdayWednesday",
  "common:weekdayThursday",
  "common:weekdayFriday",
  "common:weekdaySaturday",
] as const;

const DESCRIPTORS = new Set([
  "@hourly",
  "@daily",
  "@midnight",
  "@weekly",
  "@monthly",
  "@yearly",
  "@annually",
]);
const EVERY_RE = /^@every\s+(\d+[hms])+$/;

// The month and day-of-week fields accept names as well as numbers, which the
// scheduler's parser (robfig/cron, with Month and Dow enabled) resolves
// case-insensitively. A numbers-only check rejected "0 9 * * MON-FRI" — an
// expression the scheduler runs happily — leaving the editor stricter than the
// thing that actually executes the schedule.
const MONTH_NAMES = "jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec";
const DOW_NAMES = "sun|mon|tue|wed|thu|fri|sat";

/**
 * One cron field: a star, a value, or a range — each optionally stepped, and
 * any of them comma-joined. `names` widens "a value" to that field's spelled
 * forms.
 */
function cronFieldPattern(names?: string): RegExp {
  const value = names ? `(?:\\d+|${names})` : "\\d+";
  const term = `(?:\\*|${value}(?:-${value})?)(?:/\\d+)?`;
  return new RegExp(`^${term}(?:,${term})*$`, "i");
}

// Positional, because the fields do not accept the same things: names are
// month/dow only, and "?" (an alias for "*") is day-of-month/day-of-week only.
const CRON_FIELD_RES = [
  cronFieldPattern(),
  cronFieldPattern(),
  cronFieldPattern(),
  cronFieldPattern(MONTH_NAMES),
  cronFieldPattern(DOW_NAMES),
];
const ANY_DAY_FIELD_INDEXES = new Set([2, 4]);

export function isValidExpression(expression: string): boolean {
  const trimmed = expression.trim();
  if (!trimmed) return true;
  if (DESCRIPTORS.has(trimmed) || EVERY_RE.test(trimmed)) return true;
  const fields = trimmed.split(/\s+/);
  if (fields.length !== 5) return false;
  return fields.every((field, index) => {
    if (field === "?") return ANY_DAY_FIELD_INDEXES.has(index);
    return CRON_FIELD_RES[index].test(field);
  });
}

const pad = (value: number) => String(value).padStart(2, "0");

/**
 * What the preview should describe. It follows what is actually stored rather
 * than the defaults the controls fall back to: a trigger with no expression
 * does not fire (the scheduler skips an empty cron), so announcing a next run
 * for one would be a promise the automation will not keep.
 */
function previewFor(
  frequency: ScheduleFrequency,
  customDraft: string,
  storedExpression: string,
  expression: string,
): string {
  if (frequency === "none") return "";
  if (frequency === "custom") return customDraft;
  return storedExpression ? expression : "";
}

function activeFrequency(
  hasStoredSchedule: boolean,
  customMode: boolean,
  parsedFrequency: Frequency,
): ScheduleFrequency {
  if (customMode) return "custom";
  if (!hasStoredSchedule) return "none";
  return parsedFrequency;
}

function activeExpression(
  frequency: ScheduleFrequency,
  customDraft: string,
  parsed: ScheduleSpec,
): string {
  if (frequency === "none") return "";
  if (frequency === "custom") return customDraft;
  return buildExpression(parsed);
}

/**
 * The "how often is this actually checked" note.
 *
 * A bare icon under `asChild` is neither focusable nor named, so the
 * explanation was unreachable by keyboard and invisible to a screen reader.
 * The button is what makes it both.
 */
function SchedulePollingHint() {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={t("automations:aboutSchedulePolling")}
          className="relative inline-flex size-5 shrink-0 cursor-help items-center justify-center text-muted-foreground outline-none after:absolute after:-inset-2 hover:text-foreground focus-visible:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          <IconInfoCircle className="h-3.5 w-3.5" aria-hidden="true" />
        </button>
      </TooltipTrigger>
      <TooltipContent className="max-w-[280px]">
        {t("automations:schedulePollingHelp")}
      </TooltipContent>
    </Tooltip>
  );
}

export function ScheduleSelector({ config, isDirty = false, onChange }: ScheduleSelectorProps) {
  const { t } = useTranslation();
  const storedExpression = (config?.cron_expression as string) ?? "";
  const storedTimezone = (config?.timezone as string) ?? "";
  const hasStoredSchedule = Boolean(storedExpression.trim());

  const parsed = useMemo(() => parseExpression(storedExpression), [storedExpression]);
  const [customMode, setCustomMode] = useState(hasStoredSchedule && parsed.frequency === "custom");
  const [customDraft, setCustomDraft] = useState(parsed.expression);
  const [customError, setCustomError] = useState<string | null>(null);

  // Re-sync when the saved config arrives late or changes elsewhere.
  useEffect(() => {
    const next = parseExpression(storedExpression);
    setCustomDraft(next.expression);
    setCustomMode(hasStoredSchedule && next.frequency === "custom");
  }, [hasStoredSchedule, storedExpression]);

  const frequency = activeFrequency(hasStoredSchedule, customMode, parsed.frequency);

  // A schedule that has never been saved adopts the viewer's own timezone. One
  // that was saved without a timezone stays on UTC — the backend has been
  // running it that way, and silently relocalising it would move every existing
  // automation by the offset without anyone asking for it.
  const isNewSchedule = !storedExpression;
  const timezone = storedTimezone || (isNewSchedule ? browserTimeZone() : "");
  const expression = activeExpression(frequency, customDraft, parsed);
  const wallClock = usesTimezone(expression);

  const previewExpression = previewFor(frequency, customDraft, storedExpression, expression);

  const emit = (spec: ScheduleSpec, nextTimezone: string) => {
    const nextExpression = buildExpression(spec);
    onChange({
      cron_expression: nextExpression,
      // An interval carries no timezone. Sending undefined removes the key
      // through the merge in TriggersSection rather than leaving a stale one.
      timezone: usesTimezone(nextExpression) ? nextTimezone || undefined : undefined,
    });
  };

  /** The spec currently on screen, including an in-progress custom draft. */
  const currentSpec = (): ScheduleSpec => ({
    ...parsed,
    frequency: frequency === "none" ? parsed.frequency : frequency,
    expression: frequency === "custom" ? customDraft.trim() : parsed.expression,
  });

  const handleFrequency = (value: ScheduleFrequency) => {
    setCustomError(null);
    if (value === "none") {
      setCustomMode(false);
      return;
    }
    if (value === "custom") {
      // Seed the field with the schedule already in effect, so opening the
      // escape hatch never changes when the automation runs.
      setCustomMode(true);
      setCustomDraft(buildExpression(parsed));
      return;
    }
    setCustomMode(false);
    emit({ ...parsed, frequency: value }, timezone);
  };

  const handleCustomBlur = () => {
    const trimmed = customDraft.trim();
    if (!isValidExpression(trimmed)) {
      setCustomError(t("automations:scheduleUnparseable"));
      return;
    }
    setCustomError(null);
    emit({ ...parsed, frequency: "custom", expression: trimmed }, timezone);
  };

  return (
    <div className="space-y-2" data-testid="schedule-selector">
      <div className="flex items-center gap-2 flex-wrap text-sm">
        <span className="text-muted-foreground">{t("automations:run")}</span>

        <FrequencySelect value={frequency} onChange={handleFrequency} />

        <FrequencyDetail
          frequency={frequency}
          spec={parsed}
          isDirty={isDirty}
          customDraft={customDraft}
          hasCustomError={!!customError}
          onSpecChange={(next) => emit(next, timezone)}
          onCustomDraftChange={(value) => {
            setCustomDraft(value);
            if (customError) setCustomError(null);
          }}
          onCustomBlur={handleCustomBlur}
        />

        {wallClock && (
          <TimezoneField value={timezone} onChange={(zone) => emit(currentSpec(), zone)} />
        )}

        <SchedulePollingHint />
      </div>

      {customError && (
        <p className="text-xs text-destructive" data-testid="schedule-error">
          {customError}
        </p>
      )}

      <NextRun
        expression={previewExpression}
        timezone={timezone}
        showTimezonePrompt={wallClock && !storedTimezone && !isNewSchedule}
        onAdoptBrowserTimezone={() => emit(currentSpec(), browserTimeZone())}
      />
    </div>
  );
}

function FrequencySelect({
  value,
  onChange,
}: {
  value: ScheduleFrequency;
  onChange: (value: ScheduleFrequency) => void;
}) {
  const { t } = useTranslation();
  const options =
    value === "none"
      ? FREQUENCY_OPTIONS
      : FREQUENCY_OPTIONS.filter((option) => option.value !== "none");

  return (
    <Select value={value} onValueChange={(next) => onChange(next as ScheduleFrequency)}>
      <SelectTrigger className="w-[170px] h-8" data-testid="schedule-frequency">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {t(option.labelKey)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function FrequencyDetail({
  frequency,
  spec,
  isDirty,
  customDraft,
  hasCustomError,
  onSpecChange,
  onCustomDraftChange,
  onCustomBlur,
}: {
  frequency: ScheduleFrequency;
  spec: ScheduleSpec;
  isDirty: boolean;
  customDraft: string;
  hasCustomError: boolean;
  onSpecChange: (spec: ScheduleSpec) => void;
  onCustomDraftChange: (value: string) => void;
  onCustomBlur: () => void;
}) {
  const { t } = useTranslation();

  if (frequency === "none") return null;

  if (frequency === "custom") {
    return (
      <Input
        value={customDraft}
        onChange={(event) => onCustomDraftChange(event.target.value)}
        onBlur={onCustomBlur}
        data-testid="schedule-custom-input"
        data-settings-dirty={isDirty}
        placeholder="0 9 * * 1-5"
        className={`font-mono h-8 w-[170px] ${hasCustomError ? "border-destructive" : ""}`}
      />
    );
  }

  if (frequency === "hourly") {
    return (
      <>
        <span className="text-muted-foreground">{t("automations:atMinute")}</span>
        <Input
          type="number"
          min={0}
          max={59}
          value={spec.minute}
          onChange={(event) => {
            const minute = Number(event.target.value);
            if (!Number.isFinite(minute)) return;
            onSpecChange({ ...spec, minute: Math.min(59, Math.max(0, minute)) });
          }}
          data-testid="schedule-minute"
          data-settings-dirty={isDirty}
          className="w-[75px] h-8"
        />
      </>
    );
  }

  if (frequency !== "daily" && frequency !== "weekly") return null;

  return (
    <>
      {frequency === "weekly" && (
        <>
          <span className="text-muted-foreground">on</span>
          <Select
            value={String(spec.weekday)}
            onValueChange={(value) => onSpecChange({ ...spec, weekday: Number(value) })}
          >
            <SelectTrigger className="w-[125px] h-8" data-testid="schedule-weekday">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {WEEKDAY_KEYS.map((dayKey, index) => (
                <SelectItem key={dayKey} value={String(index)}>
                  {t(dayKey)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </>
      )}
      <span className="text-muted-foreground">{t("automations:scheduleAtSeparator")}</span>
      <Input
        type="time"
        value={`${pad(spec.hour)}:${pad(spec.minute)}`}
        onChange={(event) => {
          const [hour, minute] = event.target.value.split(":").map(Number);
          if (!Number.isFinite(hour) || !Number.isFinite(minute)) return;
          onSpecChange({ ...spec, hour, minute });
        }}
        data-testid="schedule-time"
        data-settings-dirty={isDirty}
        className="w-[110px] h-8"
      />
    </>
  );
}

function NextRun({
  expression,
  timezone,
  showTimezonePrompt,
  onAdoptBrowserTimezone,
}: {
  expression: string;
  timezone: string;
  showTimezonePrompt: boolean;
  onAdoptBrowserTimezone: () => void;
}) {
  const { t } = useTranslation();
  if (!expression.trim()) {
    return (
      <p className="text-xs text-muted-foreground" data-testid="schedule-next-run">
        {t("automations:noSchedule")}
      </p>
    );
  }

  // An interval is anchored server-side to the last run, so there is no instant
  // to promise here. The sentence above already says everything there is.
  if (!usesTimezone(expression)) return null;

  const zone = timezone || "UTC";
  const next = nextRun(expression, zone);
  if (!next) return null;

  const local = formatInZone(next, zone);
  const utc = formatInZone(next, "UTC");

  return (
    <p className="text-xs text-muted-foreground" data-testid="schedule-next-run">
      <span className="text-foreground/70">{t("automations:nextRun")}</span> {local}{" "}
      {timeZoneAbbreviation(zone, next)}
      {local !== utc && <span className="text-muted-foreground/70"> · {utc} UTC</span>}
      {showTimezonePrompt && (
        <>
          {" - "}
          <button
            type="button"
            onClick={onAdoptBrowserTimezone}
            className="cursor-pointer underline underline-offset-2 hover:text-foreground"
            data-testid="schedule-adopt-timezone"
          >
            {t("automations:interpretedAsUtcSetTimezone")}
          </button>
        </>
      )}
    </p>
  );
}

function TimezoneField({ value, onChange }: { value: string; onChange: (zone: string) => void }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const zones = useMemo(() => timeZoneOptions(), []);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          role="combobox"
          className="h-8 cursor-pointer font-normal justify-between gap-1"
          data-testid="schedule-timezone"
        >
          {value || "UTC"}
          <IconSelector className="h-3.5 w-3.5 text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[280px] p-0" align="start">
        <Command>
          <CommandInput placeholder={t("automations:searchTimezones")} />
          <CommandList>
            <CommandEmpty>{t("automations:noTimezoneFound")}</CommandEmpty>
            {zones.map((zone) => (
              <CommandItem
                key={zone}
                value={zone}
                onSelect={() => {
                  onChange(zone);
                  setOpen(false);
                }}
              >
                {zone}
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
