"use client";

import { useLayoutEffect, useState } from "react";
import { IconCode } from "@tabler/icons-react";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { useTranslation } from "react-i18next";

const AUTO_SHELL = "auto";
const CUSTOM_SHELL = "custom";

type ShellOption = { value: string; label: string };

function resolveShellSelection(preferredShell: string, shellOptions: ShellOption[]) {
  if (!preferredShell) {
    return { selection: AUTO_SHELL, customShell: "" };
  }
  if (shellOptions.some((option) => option.value === preferredShell)) {
    return { selection: preferredShell, customShell: "" };
  }
  return { selection: CUSTOM_SHELL, customShell: preferredShell };
}

type ShellSelectProps = {
  shellSelection: string;
  isDirty: boolean;
  onSelectionChange: (value: string) => void;
  customShell: string;
  onCustomShellChange: (value: string) => void;
  shellLoaded: boolean;
  shellOptions: ShellOption[];
};

function ShellSelect({
  shellSelection,
  isDirty,
  onSelectionChange,
  customShell,
  onCustomShellChange,
  shellLoaded,
  shellOptions,
}: ShellSelectProps) {
  const { t } = useTranslation();
  return (
    <>
      <div className="space-y-2">
        <Select
          value={shellSelection}
          onValueChange={onSelectionChange}
          disabled={!shellLoaded || shellOptions.length === 0}
        >
          <SelectTrigger data-settings-dirty={isDirty}>
            <SelectValue
              placeholder={
                shellOptions.length === 0
                  ? t("settings:shellOptionsUnavailable")
                  : t("settings:selectAShell")
              }
            />
          </SelectTrigger>
          <SelectContent>
            {shellOptions
              .filter(
                (option) =>
                  option.value !== AUTO_SHELL &&
                  option.value !== CUSTOM_SHELL &&
                  option.value !== "",
              )
              .map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            <SelectItem value={CUSTOM_SHELL}>{t("settings:custom")}</SelectItem>
            <SelectItem value={AUTO_SHELL}>{t("settings:systemDefault")}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      {shellSelection === CUSTOM_SHELL && (
        <div className="space-y-2">
          <Input
            value={customShell}
            data-settings-dirty={isDirty}
            onChange={(event) => onCustomShellChange(event.target.value)}
            placeholder="/bin/zsh"
          />
          <p className="text-xs text-muted-foreground">{t("settings:enterAShellPathOrCommand")}</p>
        </div>
      )}
    </>
  );
}

export function ShellSettingsCard({
  preferredShell,
  isDirty,
  onPreferredShellChange,
  shellLoaded,
  shellOptions,
}: {
  preferredShell: string;
  isDirty?: boolean;
  onPreferredShellChange: (value: string) => void;
  shellLoaded: boolean;
  shellOptions: ShellOption[];
}) {
  const { t } = useTranslation();
  const initialSelection = resolveShellSelection(preferredShell, shellOptions);
  const [shellSelection, setShellSelection] = useState(initialSelection.selection);
  const [customShell, setCustomShell] = useState(initialSelection.customShell);

  useLayoutEffect(() => {
    const next = resolveShellSelection(preferredShell, shellOptions);
    setShellSelection(next.selection);
    setCustomShell(next.customShell);
  }, [preferredShell, shellOptions]);

  return (
    <SettingsSection
      icon={<IconCode className="h-5 w-5" />}
      title={t("common:shell")}
      description={t("settings:pickTheDefaultShellForTask")}
    >
      <SettingsCard isDirty={isDirty}>
        <CardHeader>
          <CardTitle className="text-base">{t("settings:preferredShell")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <ShellSelect
            shellSelection={shellSelection}
            isDirty={Boolean(isDirty)}
            onSelectionChange={(value) => {
              setShellSelection(value);
              if (value === AUTO_SHELL) {
                onPreferredShellChange("");
                setCustomShell("");
                return;
              }
              if (value === CUSTOM_SHELL) {
                onPreferredShellChange(customShell);
                return;
              }
              onPreferredShellChange(value);
              setCustomShell("");
            }}
            customShell={customShell}
            onCustomShellChange={(value) => {
              setCustomShell(value);
              onPreferredShellChange(value);
            }}
            shellLoaded={shellLoaded}
            shellOptions={shellOptions}
          />
          <p className="text-xs text-muted-foreground">
            {t("settings:newTaskSessionsWillUseThis")}
          </p>
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}
