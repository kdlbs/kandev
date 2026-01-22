'use client';

import { useState } from 'react';
import { IconCode } from '@tabler/icons-react';
import { Card, CardContent, CardHeader, CardTitle } from '@kandev/ui/card';
import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import { SettingsSection } from '@/components/settings/settings-section';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { useShellSettings } from '@/hooks/domains/settings/use-shell-settings';
import { updateUserSettings } from '@/lib/api';
import { useRequest } from '@/lib/http/use-request';

const AUTO_SHELL = 'auto';
const CUSTOM_SHELL = 'custom';

type ShellOption = { value: string; label: string };

function resolveShellSelection(preferredShell: string, shellOptions: ShellOption[]) {
  if (!preferredShell) {
    return { selection: AUTO_SHELL, customShell: '' };
  }
  if (shellOptions.some((option) => option.value === preferredShell)) {
    return { selection: preferredShell, customShell: '' };
  }
  return { selection: CUSTOM_SHELL, customShell: preferredShell };
}

export function ShellSettingsCard() {
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const shellSettings = useShellSettings();
  const initialShellValue = shellSettings.preferredShell ?? '';
  const initialShellOptions = shellSettings.shellOptions ?? [];
  const initialSelection = resolveShellSelection(initialShellValue, initialShellOptions);
  const [preferredShell, setPreferredShell] = useState(initialShellValue);
  const [baselineShell, setBaselineShell] = useState(initialShellValue);
  const [shellSelection, setShellSelection] = useState(initialSelection.selection);
  const [customShell, setCustomShell] = useState(initialSelection.customShell);
  const [shellLoaded] = useState(shellSettings.loaded);
  const [shellOptions] = useState<ShellOption[]>(initialShellOptions);

  const saveShellRequest = useRequest(async () => {
    const trimmed = preferredShell.trim();
    const currentSettings = storeApi.getState().userSettings;
    await updateUserSettings({
      workspace_id: currentSettings.workspaceId ?? '',
      board_id: currentSettings.boardId ?? '',
      repository_ids: currentSettings.repositoryIds,
      preferred_shell: trimmed,
    });
    setBaselineShell(trimmed);
    setUserSettings({
      ...currentSettings,
      preferredShell: trimmed || null,
      loaded: true,
    });
  });

  // Store data is SSR-hydrated for settings. Local state is initialized once.

  const shellDirty = preferredShell.trim() !== baselineShell.trim();

  return (
    <SettingsSection
      icon={<IconCode className="h-5 w-5" />}
      title="Shell"
      description="Pick the default shell for task sessions"
    >
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-4">
          <CardTitle className="text-base">Preferred Shell</CardTitle>
          <Button
            type="button"
            onClick={() => saveShellRequest.run()}
            disabled={!shellLoaded || !shellDirty || saveShellRequest.isLoading}
          >
            {saveShellRequest.isLoading ? 'Saving...' : 'Save'}
          </Button>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Select
              value={shellSelection}
              onValueChange={(value) => {
                setShellSelection(value);
                if (value === AUTO_SHELL) {
                  setPreferredShell('');
                  setCustomShell('');
                  return;
                }
                if (value === CUSTOM_SHELL) {
                  setPreferredShell(customShell);
                  return;
                }
                setPreferredShell(value);
                setCustomShell('');
              }}
              disabled={!shellLoaded || shellOptions.length === 0}
            >
              <SelectTrigger>
                <SelectValue
                  placeholder={
                    shellOptions.length === 0 ? 'Shell options unavailable' : 'Select a shell'
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {shellOptions
                  .filter(
                    (option) =>
                      option.value !== AUTO_SHELL &&
                      option.value !== CUSTOM_SHELL &&
                      option.value !== ''
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
                onChange={(event) => {
                  const nextValue = event.target.value;
                  setCustomShell(nextValue);
                  setPreferredShell(nextValue);
                }}
                placeholder="/bin/zsh"
              />
              <p className="text-xs text-muted-foreground">
                Enter a shell path or command available in the agent environment.
              </p>
            </div>
          )}
          <p className="text-xs text-muted-foreground">
            New task sessions will use this shell. Existing sessions keep their current shell.
          </p>
        </CardContent>
      </Card>
    </SettingsSection>
  );
}
