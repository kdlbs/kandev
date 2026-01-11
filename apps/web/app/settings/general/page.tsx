'use client';

import { useState, useSyncExternalStore } from 'react';
import { useTheme } from 'next-themes';
import { IconPalette, IconCode, IconBell, IconServer } from '@tabler/icons-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { SettingsSection } from '@/components/settings/settings-section';
import { SETTINGS_DATA } from '@/lib/settings/dummy-data';
import { getBackendConfig } from '@/lib/config';
import type { Theme, Editor, Notifications } from '@/lib/settings/types';

export default function GeneralSettingsPage() {
  const { theme: currentTheme, setTheme } = useTheme();
  const [editor, setEditor] = useState<Editor>(SETTINGS_DATA.general.editor);
  const [customEditorCommand, setCustomEditorCommand] = useState<string>(
    SETTINGS_DATA.general.customEditorCommand || ''
  );
  const [notifications, setNotifications] = useState<Notifications>(
    SETTINGS_DATA.general.notifications
  );
  const [backendUrl] = useState<string>(() => getBackendConfig().apiBaseUrl);
  const mounted = useSyncExternalStore(
    () => () => undefined,
    () => true,
    () => false
  );

  const handleNotificationToggle = (key: keyof Notifications) => {
    setNotifications((prev) => ({ ...prev, [key]: !prev[key] }));
  };

  const displayBackendUrl = backendUrl.replace(/^https?:\/\//, '').replace(/\/$/, '');

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
        icon={<IconBell className="h-5 w-5" />}
        title="Notifications"
        description="Control what notifications you receive"
      >
        <Card>
          <CardContent className="pt-6">
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <Label htmlFor="task-updates" className="text-base font-medium">
                    Task Updates
                  </Label>
                  <p className="text-sm text-muted-foreground">
                    Get notified when tasks change status
                  </p>
                </div>
                <button
                  id="task-updates"
                  role="switch"
                  aria-checked={notifications.taskUpdates}
                  onClick={() => handleNotificationToggle('taskUpdates')}
                  className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                    notifications.taskUpdates ? 'bg-primary' : 'bg-muted'
                  }`}
                >
                  <span
                    className={`inline-block h-4 w-4 transform rounded-full bg-background transition-transform ${
                      notifications.taskUpdates ? 'translate-x-6' : 'translate-x-1'
                    }`}
                  />
                </button>
              </div>

              <Separator />

              <div className="flex items-center justify-between">
                <div>
                  <Label htmlFor="agent-completion" className="text-base font-medium">
                    Agent Completion
                  </Label>
                  <p className="text-sm text-muted-foreground">
                    Get notified when agents complete tasks
                  </p>
                </div>
                <button
                  id="agent-completion"
                  role="switch"
                  aria-checked={notifications.agentCompletion}
                  onClick={() => handleNotificationToggle('agentCompletion')}
                  className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                    notifications.agentCompletion ? 'bg-primary' : 'bg-muted'
                  }`}
                >
                  <span
                    className={`inline-block h-4 w-4 transform rounded-full bg-background transition-transform ${
                      notifications.agentCompletion ? 'translate-x-6' : 'translate-x-1'
                    }`}
                  />
                </button>
              </div>

              <Separator />

              <div className="flex items-center justify-between">
                <div>
                  <Label htmlFor="errors" className="text-base font-medium">
                    Errors
                  </Label>
                  <p className="text-sm text-muted-foreground">
                    Get notified about errors and failures
                  </p>
                </div>
                <button
                  id="errors"
                  role="switch"
                  aria-checked={notifications.errors}
                  onClick={() => handleNotificationToggle('errors')}
                  className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                    notifications.errors ? 'bg-primary' : 'bg-muted'
                  }`}
                >
                  <span
                    className={`inline-block h-4 w-4 transform rounded-full bg-background transition-transform ${
                      notifications.errors ? 'translate-x-6' : 'translate-x-1'
                    }`}
                  />
                </button>
              </div>
            </div>
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
