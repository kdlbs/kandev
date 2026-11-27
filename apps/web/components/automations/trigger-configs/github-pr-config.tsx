"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { RepoFilterSelector } from "@/components/github/repo-filter-selector";
import { SettingsInfo } from "@/components/settings/settings-info";
import type { RepoFilter } from "@/lib/types/github";
import { PR_EVENTS } from "./pr-events";

type GitHubPRConfigProps = {
  config: Record<string, unknown>;
  workspaceId: string;
  onUpdate: (config: Record<string, unknown>) => void;
};

const MAX_QUIET_PERIOD_MINUTES = 1440;

function parseCommaList(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

export function GitHubPRConfig({ config, workspaceId, onUpdate }: GitHubPRConfigProps) {
  const { t } = useTranslation();
  const events = (config.events as string[]) ?? [];
  const repos = (config.repos as RepoFilter[]) ?? [];
  const allRepos = (config.all_repos as boolean) ?? true;
  const configBranches = ((config.branches as string[]) ?? []).join(", ");
  const configAuthors = ((config.authors as string[]) ?? []).join(", ");
  const [branches, setBranches] = useState(configBranches);
  const [authors, setAuthors] = useState(configAuthors);
  useEffect(() => {
    setBranches(configBranches);
  }, [configBranches]);
  useEffect(() => {
    setAuthors(configAuthors);
  }, [configAuthors]);
  const excludeDraft = (config.exclude_draft as boolean) ?? false;

  const toggleEvent = (event: string) => {
    const next = events.includes(event) ? events.filter((e) => e !== event) : [...events, event];
    onUpdate({ ...config, events: next });
  };

  const handleBranchesBlur = () => {
    onUpdate({ ...config, branches: parseCommaList(branches) });
  };

  const handleAuthorsBlur = () => {
    onUpdate({ ...config, authors: parseCommaList(authors) });
  };

  return (
    <div className="space-y-3">
      <EventsSection events={events} onToggle={toggleEvent} />
      {events.includes("opened") && <IncludeExistingInput config={config} onUpdate={onUpdate} />}
      {events.includes("new_commits") && <QuietPeriodInput config={config} onUpdate={onUpdate} />}
      {events.includes("review_requested") && (
        <ReviewersInput config={config} onUpdate={onUpdate} />
      )}
      <RepoFilterSelector
        allRepos={allRepos}
        selectedRepos={repos}
        workspaceId={workspaceId}
        onAllReposChange={(checked) => {
          onUpdate({ ...config, all_repos: checked, repos: checked ? [] : repos });
        }}
        onSelectedReposChange={(next) => onUpdate({ ...config, repos: next })}
      />
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label className="text-xs">{t("automations:baseBranchesLabel")}</Label>
          {/* Example branch names — data the user types verbatim, not copy. */}
          <Input
            value={branches}
            onChange={(e) => setBranches(e.target.value)}
            onBlur={handleBranchesBlur}
            // eslint-disable-next-line i18next/no-literal-string -- example branch names, see above
            placeholder="main, develop"
          />
        </div>
        <div className="space-y-1.5">
          <Label className="text-xs">{t("automations:authorsLabel")}</Label>
          {/* Example GitHub usernames — data, not copy. */}
          <Input
            value={authors}
            onChange={(e) => setAuthors(e.target.value)}
            onBlur={handleAuthorsBlur}
            // eslint-disable-next-line i18next/no-literal-string -- example usernames, see above
            placeholder="alice, bob"
          />
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Switch
          size="sm"
          checked={excludeDraft}
          onCheckedChange={(checked) => onUpdate({ ...config, exclude_draft: checked })}
          className="cursor-pointer"
        />
        <Label className="text-xs">{t("automations:excludeDraftPrs")}</Label>
      </div>
    </div>
  );
}

// Rows stay 44 px tall on phones, tablets, and any coarse pointer.
const TOUCH_ROW_CLASS_NAME = "min-h-11 md:[@media(pointer:fine)]:min-h-0";

type FieldProps = {
  config: Record<string, unknown>;
  onUpdate: (config: Record<string, unknown>) => void;
};

function IncludeExistingInput({ config, onUpdate }: FieldProps) {
  const { t } = useTranslation();
  const checked = (config.include_existing_prs as boolean) ?? false;
  return (
    <div className="space-y-1">
      <label className={`flex items-center gap-1.5 cursor-pointer ${TOUCH_ROW_CLASS_NAME}`}>
        <Checkbox
          checked={checked}
          onCheckedChange={(value) => onUpdate({ ...config, include_existing_prs: value === true })}
          aria-describedby="github-pr-include-existing-help"
          className="cursor-pointer"
        />
        <span className="text-sm">{t("automations:prIncludeExistingLabel")}</span>
      </label>
      <p id="github-pr-include-existing-help" className="text-xs text-muted-foreground">
        {t("automations:prIncludeExistingHelp")}
      </p>
    </div>
  );
}

function QuietPeriodInput({ config, onUpdate }: FieldProps) {
  const { t } = useTranslation();
  const saved = String((config.quiet_period_minutes as number) ?? 0);
  const [value, setValue] = useState(saved);
  useEffect(() => {
    setValue(saved);
  }, [saved]);

  const handleBlur = () => {
    const minutes = Number(value);
    const valid =
      value.trim() !== "" &&
      Number.isInteger(minutes) &&
      minutes >= 0 &&
      minutes <= MAX_QUIET_PERIOD_MINUTES;
    if (!valid) {
      setValue(saved);
      return;
    }
    if (String(minutes) !== saved) onUpdate({ ...config, quiet_period_minutes: minutes });
  };

  return (
    <div className="space-y-1.5">
      <Label htmlFor="github-pr-quiet-period" className="text-xs">
        {t("automations:prQuietPeriodLabel")}
      </Label>
      <div className="flex items-center gap-2">
        <Input
          id="github-pr-quiet-period"
          type="number"
          inputMode="numeric"
          min={0}
          max={MAX_QUIET_PERIOD_MINUTES}
          step={1}
          className="w-24"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onBlur={handleBlur}
          aria-describedby="github-pr-quiet-period-help"
        />
        <span className="text-xs text-muted-foreground">{t("automations:prQuietPeriodUnit")}</span>
      </div>
      <p id="github-pr-quiet-period-help" className="text-xs text-muted-foreground">
        {t("automations:prQuietPeriodHelp")}
      </p>
    </div>
  );
}

function ReviewersInput({ config, onUpdate }: FieldProps) {
  const { t } = useTranslation();
  const saved = ((config.reviewers as string[]) ?? []).join(", ");
  const [value, setValue] = useState(saved);
  useEffect(() => {
    setValue(saved);
  }, [saved]);

  return (
    <div className="space-y-1.5">
      <Label htmlFor="github-pr-reviewers" className="text-xs">
        {t("automations:prReviewersLabel")}
      </Label>
      {/* Example GitHub logins and teams: data, not copy. */}
      <Input
        id="github-pr-reviewers"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onBlur={() => onUpdate({ ...config, reviewers: parseCommaList(value) })}
        aria-describedby="github-pr-reviewers-help"
        // eslint-disable-next-line i18next/no-literal-string -- example logins, see above
        placeholder="kandev-bot, acme/reviewers"
      />
      <p id="github-pr-reviewers-help" className="text-xs text-muted-foreground">
        {t("automations:prReviewersHelp")}
      </p>
    </div>
  );
}

function EventsSection({
  events,
  onToggle,
}: {
  events: string[];
  onToggle: (event: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <Label className="text-xs">{t("automations:eventsLabel")}</Label>
      <div className="flex flex-wrap gap-3">
        {PR_EVENTS.map((evt) => (
          <div key={evt.value} className="flex items-center gap-0.5">
            <label className={`flex items-center gap-1.5 cursor-pointer ${TOUCH_ROW_CLASS_NAME}`}>
              <Checkbox
                checked={events.includes(evt.value)}
                onCheckedChange={() => onToggle(evt.value)}
                className="cursor-pointer"
              />
              <span className="text-sm">{t(evt.labelKey)}</span>
            </label>
            <SettingsInfo label={t(evt.labelKey)}>{t(evt.tooltipKey)}</SettingsInfo>
          </div>
        ))}
      </div>
    </div>
  );
}
