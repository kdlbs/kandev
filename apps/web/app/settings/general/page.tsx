'use client';

import { useEffect, useState, useSyncExternalStore } from 'react';
import { useTheme } from 'next-themes';
import { IconPalette, IconCode, IconServer } from '@tabler/icons-react';
import { Card, CardContent, CardHeader, CardTitle } from '@kandev/ui/card';
import { Button } from '@kandev/ui/button';
import { Label } from '@kandev/ui/label';
import { Input } from '@kandev/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import { Separator } from '@kandev/ui/separator';
import { SettingsSection } from '@/components/settings/settings-section';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { fetchUserSettings, updateUserSettings } from '@/lib/http';
import { useRequest } from '@/lib/http/use-request';
import { SETTINGS_DATA } from '@/lib/settings/dummy-data';
import { getBackendConfig } from '@/lib/config';
import type { Theme, Editor } from '@/lib/settings/types';

export default function GeneralSettingsPage() {
  const { theme: currentTheme, setTheme } = useTheme();
  const [editor, setEditor] = useState<Editor>(SETTINGS_DATA.general.editor);
  const [customEditorCommand, setCustomEditorCommand] = useState<string>(
    SETTINGS_DATA.general.customEditorCommand || ''
  );
  const [backendUrl] = useState<string>(() => getBackendConfig().apiBaseUrl);
  const [preferredShell, setPreferredShell] = useState('');
  const [customShell, setCustomShell] = useState('');
  const [shellSelection, setShellSelection] = useState('auto');
  const [baselineShell, setBaselineShell] = useState('');
  const [shellLoaded, setShellLoaded] = useState(false);
  const [shellOptions, setShellOptions] = useState<Array<{ value: string; label: string }>>([]);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
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
  const mounted = useSyncExternalStore(
    () => () => undefined,
    () => true,
    () => false
  );

  const displayBackendUrl = backendUrl.replace(/^https?:\/\//, '').replace(/\/$/, '');
  const shellDirty = preferredShell.trim() !== baselineShell.trim();

  useEffect(() => {
    let isActive = true;
    fetchUserSettings({ cache: 'no-store' })
      .then((data) => {
        if (!isActive || !data?.settings) return;
        const shellValue = data.settings.preferred_shell || '';
        setPreferredShell(shellValue);
        setBaselineShell(shellValue);
        const nextShellOptions = data.shell_options ?? [];
        setShellOptions(nextShellOptions);
        if (shellValue === '') {
          setShellSelection('auto');
          setCustomShell('');
        } else if (nextShellOptions.some((option) => option.value === shellValue)) {
          setShellSelection(shellValue);
          setCustomShell('');
        } else {
          setShellSelection('custom');
          setCustomShell(shellValue);
        }
        setShellLoaded(true);
        const currentSettings = storeApi.getState().userSettings;
        setUserSettings({
          ...currentSettings,
          preferredShell: shellValue || null,
          loaded: true,
        });
      })
      .catch(() => {
        if (isActive) {
          setShellLoaded(true);
        }
      });
    return () => {
      isActive = false;
    };
  }, [setUserSettings, storeApi]);

  if (!mounted) {
    return null;
  }

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">General Settings</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Manage your application preferences and notifications
        </p>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconPalette className="h-5 w-5" />}
        title="Appearance"
        description="Customize how the application looks"
      >
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Theme</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              <Label htmlFor="theme">Color Theme</Label>
              <Select
                value={currentTheme}
                onValueChange={(value) => setTheme(value as Theme)}
              >
                <SelectTrigger id="theme">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="system">System</SelectItem>
                  <SelectItem value="light">Light</SelectItem>
                  <SelectItem value="dark">Dark</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </CardContent>
        </Card>
      </SettingsSection>

      <Separator />

      <SettingsSection
        icon={<IconCode className="h-5 w-5" />}
        title="Editor"
        description="Choose your preferred code editor"
      >
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Editor Preference</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="editor">Default Editor</Label>
              <Select value={editor} onValueChange={(value) => setEditor(value as Editor)}>
                <SelectTrigger id="editor">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="vscode">VS Code</SelectItem>
                  <SelectItem value="cursor">Cursor</SelectItem>
                  <SelectItem value="zed">Zed</SelectItem>
                  <SelectItem value="vim">Vim</SelectItem>
                  <SelectItem value="custom">Custom</SelectItem>
                </SelectContent>
              </Select>
            </div>

            {editor === 'custom' && (
              <div className="space-y-2">
                <Label htmlFor="custom-command">Custom Editor Command</Label>
                <Input
                  id="custom-command"
                  value={customEditorCommand}
                  onChange={(e) => setCustomEditorCommand(e.target.value)}
                  placeholder="code --goto {file}:{line}"
                />
                <p className="text-xs text-muted-foreground">
                  Use {'{file}'} and {'{line}'} as placeholders
                </p>
              </div>
            )}
          </CardContent>
        </Card>
      </SettingsSection>

      <Separator />

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
                  if (value === 'auto') {
                    setPreferredShell('');
                    setCustomShell('');
                    return;
                  }
                  if (value === 'custom') {
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
                      shellOptions.length === 0
                        ? 'Shell options unavailable'
                        : 'Select a shell'
                    }
                  />
                </SelectTrigger>
                <SelectContent>
                  {shellOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {shellSelection === 'custom' && (
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

      <Separator />

      <SettingsSection
        icon={<IconServer className="h-5 w-5" />}
        title="Backend Connection"
        description="Configure the backend server URL"
      >
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Backend Server URL</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              <Label htmlFor="backend-url">Server URL</Label>
              <Input
                id="backend-url"
                type="url"
                value={displayBackendUrl}
                readOnly
                disabled
                placeholder="http://localhost:8080"
                className="cursor-default"
              />
              <p className="text-xs text-muted-foreground">
                Backend URL is provided at runtime for SSR and WebSocket connections.
              </p>
            </div>
          </CardContent>
        </Card>
      </SettingsSection>
    </div>
  );
}
