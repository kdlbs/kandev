"use client";

import { useEffect, useState } from "react";
import { IconCode } from "@tabler/icons-react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { SettingsSection } from "@/components/settings/settings-section";

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
  onSelectionChange: (value: string) => void;
  customShell: string;
  onCustomShellChange: (value: string) => void;
  shellLoaded: boolean;
  shellOptions: ShellOption[];
};

function ShellSelect({
  shellSelection,
  onSelectionChange,
  customShell,
  onCustomShellChange,
  shellLoaded,
  shellOptions,
}: ShellSelectProps) {
  return (
    <>
      <div className="space-y-2">
        <Select
          value={shellSelection}
          onValueChange={onSelectionChange}
          disabled={!shellLoaded || shellOptions.length === 0}
        >
          <SelectTrigger>
            <SelectValue
              placeholder={
                shellOptions.length === 0 ? "Shell options unavailable" : "Select a shell"
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
            <SelectItem value={CUSTOM_SHELL}>Custom</SelectItem>
            <SelectItem value={AUTO_SHELL}>System default</SelectItem>
          </SelectContent>
        </Select>
      </div>
      {shellSelection === CUSTOM_SHELL && (
        <div className="space-y-2">
          <Input
            value={customShell}
            onChange={(event) => onCustomShellChange(event.target.value)}
            placeholder="/bin/zsh"
          />
          <p className="text-xs text-muted-foreground">
            Enter a shell path or command available in the agent environment.
          </p>
        </div>
      )}
    </>
  );
}

export function ShellSettingsCard({
  preferredShell,
  onPreferredShellChange,
  shellLoaded,
  shellOptions,
}: {
  preferredShell: string;
  onPreferredShellChange: (value: string) => void;
  shellLoaded: boolean;
  shellOptions: ShellOption[];
}) {
  const initialSelection = resolveShellSelection(preferredShell, shellOptions);
  const [shellSelection, setShellSelection] = useState(initialSelection.selection);
  const [customShell, setCustomShell] = useState(initialSelection.customShell);

  useEffect(() => {
    const next = resolveShellSelection(preferredShell, shellOptions);
    setShellSelection(next.selection);
    setCustomShell(next.customShell);
  }, [preferredShell, shellOptions]);

  return (
    <SettingsSection
      icon={<IconCode className="h-5 w-5" />}
      title="Shell"
      description="Pick the default shell for task sessions"
    >
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Preferred Shell</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <ShellSelect
            shellSelection={shellSelection}
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
            New task sessions will use this shell. Existing sessions keep their current shell.
          </p>
        </CardContent>
      </Card>
    </SettingsSection>
  );
}
