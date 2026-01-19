'use client';

import { useState } from 'react';
import { useTheme } from 'next-themes';
import { IconPalette, IconServer } from '@tabler/icons-react';
import { Card, CardContent, CardHeader, CardTitle } from '@kandev/ui/card';
import { Label } from '@kandev/ui/label';
import { Input } from '@kandev/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import { Separator } from '@kandev/ui/separator';
import { SettingsSection } from '@/components/settings/settings-section';
import { ShellSettingsCard } from '@/components/settings/shell-settings-card';
import { getBackendConfig } from '@/lib/config';
import type { Theme } from '@/lib/settings/types';

export function GeneralSettings() {
  const { theme: currentTheme, setTheme } = useTheme();
  const [backendUrl] = useState<string>(() => getBackendConfig().apiBaseUrl);
  const displayBackendUrl = backendUrl.replace(/^https?:\/\//, '').replace(/\/$/, '');
  const themeValue = currentTheme ?? 'system';

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
              <Select value={themeValue} onValueChange={(value) => setTheme(value as Theme)}>
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

      <ShellSettingsCard />

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
