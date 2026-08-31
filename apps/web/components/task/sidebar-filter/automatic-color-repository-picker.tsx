"use client";

import { IconChevronLeft, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { cn } from "@/lib/utils";
import type { SidebarTaskColorRule } from "@/lib/task-color-automation-settings";
import type { TaskColorRuleOption } from "./task-color-rule-options";
import type { RepositoryRuleCatalogOption } from "@/lib/sidebar/repository-rule-catalog";
import { useTranslation } from "react-i18next";

export type Translate = (key: string, options?: Record<string, unknown>) => string;

const GROUP_LABEL_KEYS: Record<RepositoryRuleCatalogOption["group"], string> = {
  workspace: "task:automaticColorsRepositoryWorkspace",
  local: "task:automaticColorsRepositoryLocal",
  remote: "task:automaticColorsRepositoryRemote",
  plugin: "task:automaticColorsRepositoryRemote",
  unavailable: "task:automaticColorsRepositoryUnavailable",
};

export function RepositoryConditionField({
  rule,
  selectedOption,
  isDrawerLayout,
  options,
  query,
  loading,
  error,
  onQueryChange,
  onRefresh,
  onOpen,
  onSelect,
  t,
}: {
  rule: SidebarTaskColorRule;
  selectedOption: TaskColorRuleOption | undefined;
  isDrawerLayout: boolean;
  options: readonly RepositoryRuleCatalogOption[];
  query: string;
  loading: boolean;
  error: Error | null;
  onQueryChange: (query: string) => void;
  onRefresh: () => void;
  onOpen: () => void;
  onSelect: (option: RepositoryRuleCatalogOption) => void;
  t: Translate;
}) {
  const button = (
    <Button
      type="button"
      variant="outline"
      className="min-h-11 w-full min-w-0 justify-start truncate text-xs md:min-h-0 md:h-7"
      onClick={onOpen}
      data-testid={`automatic-color-repository-trigger-${rule.id}`}
      aria-label={t("task:automaticColorsSelectRepository")}
    >
      {selectedOption?.label ?? t("task:automaticColorsSelectRepository")}
    </Button>
  );

  const field = (
    <label className="min-w-0">
      <span className="mb-1 block text-[11px] font-medium text-muted-foreground">
        {t("task:automaticColorsSelectRepository")}
      </span>
      {button}
    </label>
  );

  if (isDrawerLayout) return field;
  return (
    <Popover>
      <PopoverTrigger asChild>{button}</PopoverTrigger>
      <PopoverContent align="start" className="w-[20rem] p-2">
        <RepositoryPicker
          options={options}
          query={query}
          loading={loading}
          error={error}
          onQueryChange={onQueryChange}
          onRefresh={onRefresh}
          onSelect={onSelect}
          isDrawerLayout={false}
          t={t}
        />
      </PopoverContent>
    </Popover>
  );
}

export function RepositoryPickerPane({
  options,
  query,
  loading,
  error,
  onQueryChange,
  onRefresh,
  onBack,
  onSelect,
}: {
  options: readonly RepositoryRuleCatalogOption[];
  query: string;
  loading: boolean;
  error: Error | null;
  onQueryChange: (query: string) => void;
  onRefresh: () => void;
  onBack: () => void;
  onSelect: (option: RepositoryRuleCatalogOption) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" data-testid="automatic-color-repository-pane">
      <Button
        type="button"
        variant="ghost"
        className="min-h-11 w-full cursor-pointer justify-start px-1 text-xs"
        onClick={onBack}
        data-testid="automatic-color-repository-back"
      >
        <IconChevronLeft className="mr-1 size-4" aria-hidden="true" />
        {t("task:automaticColorsBackToRule")}
      </Button>
      <RepositoryPicker
        options={options}
        query={query}
        loading={loading}
        error={error}
        onQueryChange={onQueryChange}
        onRefresh={onRefresh}
        onSelect={onSelect}
        isDrawerLayout
        t={t}
      />
    </div>
  );
}

function RepositoryPicker({
  options,
  query,
  loading,
  error,
  onQueryChange,
  onRefresh,
  onSelect,
  isDrawerLayout,
  t,
}: {
  options: readonly RepositoryRuleCatalogOption[];
  query: string;
  loading: boolean;
  error: Error | null;
  onQueryChange: (query: string) => void;
  onRefresh: () => void;
  onSelect: (option: RepositoryRuleCatalogOption) => void;
  isDrawerLayout: boolean;
  t: Translate;
}) {
  const groups = [...new Set(options.map((option) => option.group))];
  return (
    <div className="space-y-2" data-testid="automatic-color-repository-picker">
      <div className="flex items-center gap-1">
        <Input
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          placeholder={t("task:automaticColorsSearchRepositories")}
          className="min-h-11 text-xs md:min-h-0 md:h-8"
          data-testid="automatic-color-repository-search"
        />
        <Button
          type="button"
          variant="outline"
          size="icon"
          className="size-11 shrink-0 cursor-pointer md:size-8"
          onClick={onRefresh}
          disabled={loading}
          aria-label={t("task:automaticColorsRefreshRepositories")}
          data-testid="automatic-color-repository-refresh"
        >
          <IconRefresh className={cn("size-3.5", loading && "animate-spin")} aria-hidden="true" />
        </Button>
      </div>
      {error && (
        <p className="text-[11px] text-muted-foreground" role="status">
          {t("task:automaticColorsRepositoryError")}
        </p>
      )}
      {loading && <p className="text-xs text-muted-foreground">{t("task:loading")}</p>}
      {!loading && groups.length === 0 && (
        <p className="text-xs text-muted-foreground">{t("task:automaticColorsNoRepositories")}</p>
      )}
      <div className={cn("space-y-2", !isDrawerLayout && "max-h-[18rem] overflow-y-auto")}>
        {groups.map((group) => (
          <div key={group}>
            <p className="px-1 pb-1 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
              {t(GROUP_LABEL_KEYS[group])}
            </p>
            <div className="space-y-0.5">
              {options
                .filter((option) => option.group === group)
                .map((option) => (
                  <button
                    key={option.key}
                    type="button"
                    className="flex min-h-11 w-full min-w-0 cursor-pointer items-center gap-2 rounded-md px-2 text-left text-xs hover:bg-accent disabled:cursor-not-allowed disabled:opacity-60 md:min-h-8"
                    disabled={!option.available}
                    onClick={() => onSelect(option)}
                    data-testid={`automatic-color-repository-option-${option.key}`}
                  >
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">{option.label}</span>
                      {option.secondaryLabel && (
                        <span className="block truncate text-[11px] text-muted-foreground">
                          {option.secondaryLabel}
                        </span>
                      )}
                    </span>
                    {!option.available && (
                      <span className="shrink-0 text-[10px] text-muted-foreground">
                        {t("task:automaticColorsUnavailable")}
                      </span>
                    )}
                  </button>
                ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
