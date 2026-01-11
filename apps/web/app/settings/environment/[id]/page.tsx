'use client';

import { use, useState } from 'react';
import { useRouter } from 'next/navigation';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { SETTINGS_DATA } from '@/lib/settings/dummy-data';
import type { Environment, EnvironmentType, BaseDocker, AgentType } from '@/lib/settings/types';

export default function EnvironmentEditPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const [environment, setEnvironment] = useState<Environment | undefined>(
    SETTINGS_DATA.environments.find((e) => e.id === id)
  );
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const [newEnvKey, setNewEnvKey] = useState('');
  const [newEnvValue, setNewEnvValue] = useState('');
  const [newSecretKey, setNewSecretKey] = useState('');
  const [newSecretValue, setNewSecretValue] = useState('');

  if (!environment) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Environment not found</p>
            <Button className="mt-4" onClick={() => router.push('/settings/environments')}>
              Go to Environments
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const handleAddEnvVar = () => {
    if (newEnvKey.trim()) {
      setEnvironment({
        ...environment,
        envVariables: [
          ...environment.envVariables,
          { id: crypto.randomUUID(), key: newEnvKey, value: newEnvValue },
        ],
      });
      setNewEnvKey('');
      setNewEnvValue('');
    }
  };

  const handleRemoveEnvVar = (id: string) => {
    setEnvironment({
      ...environment,
      envVariables: environment.envVariables.filter((v) => v.id !== id),
    });
  };

  const handleAddSecret = () => {
    if (newSecretKey.trim()) {
      setEnvironment({
        ...environment,
        secrets: [
          ...environment.secrets,
          { id: crypto.randomUUID(), key: newSecretKey, value: newSecretValue },
        ],
      });
      setNewSecretKey('');
      setNewSecretValue('');
    }
  };

  const handleRemoveSecret = (id: string) => {
    setEnvironment({
      ...environment,
      secrets: environment.secrets.filter((s) => s.id !== id),
    });
  };

  const handleToggleAgent = (agentId: AgentType) => {
    const installed = environment.installedAgents || [];
    if (installed.includes(agentId)) {
      setEnvironment({
        ...environment,
        installedAgents: installed.filter((id) => id !== agentId),
      });
    } else {
      setEnvironment({
        ...environment,
        installedAgents: [...installed, agentId],
      });
    }
  };

  const handleDeleteEnvironment = () => {
    if (deleteConfirmText === 'delete') {
      router.push('/settings/environments');
      setDeleteDialogOpen(false);
    }
  };

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">{environment.name}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Configure Docker environment and runtime settings
        </p>
      </div>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle>Environment Name</CardTitle>
        </CardHeader>
        <CardContent>
          <Input
            value={environment.name}
            onChange={(e) => setEnvironment({ ...environment, name: e.target.value })}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Docker Configuration</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>Environment Type</Label>
              <Select
                value={environment.type}
                onValueChange={(value) => setEnvironment({ ...environment, type: value as EnvironmentType })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="local-docker">Local Docker</SelectItem>
                  <SelectItem value="remote-docker">Remote Docker</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Base Docker Image</Label>
              <Select
                value={environment.baseDocker}
                onValueChange={(value) => setEnvironment({ ...environment, baseDocker: value as BaseDocker })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="universal">Universal</SelectItem>
                  <SelectItem value="golang">Golang</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="space-y-2">
            <Label>Setup Script</Label>
            <Textarea
              value={environment.setupScript}
              onChange={(e) => setEnvironment({ ...environment, setupScript: e.target.value })}
              placeholder="#!/bin/bash&#10;apt-get update"
              rows={5}
              className="font-mono text-sm"
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Environment Variables</CardTitle>
          <CardDescription>Key-value pairs available in the container</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            {environment.envVariables.map((envVar) => (
              <div key={envVar.id} className="flex items-center gap-2">
                <Input value={envVar.key} disabled className="flex-1" />
                <Input value={envVar.value} disabled className="flex-1" />
                <Button
                  variant="ghost"
                  onClick={() => handleRemoveEnvVar(envVar.id)}
                  className="shrink-0"
                >
                  <IconTrash className="h-4 w-4 mr-2" />
                  Delete
                </Button>
              </div>
            ))}
          </div>
          <div className="flex gap-2">
            <Input
              placeholder="KEY"
              value={newEnvKey}
              onChange={(e) => setNewEnvKey(e.target.value)}
              className="flex-1"
            />
            <Input
              placeholder="value"
              value={newEnvValue}
              onChange={(e) => setNewEnvValue(e.target.value)}
              className="flex-1"
            />
            <Button onClick={handleAddEnvVar} className="shrink-0">
              <IconPlus className="h-4 w-4 mr-2" />
              Add
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Secrets</CardTitle>
          <CardDescription>Sensitive values (not shown in plain text)</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            {environment.secrets.map((secret) => (
              <div key={secret.id} className="flex items-center gap-2">
                <Input value={secret.key} disabled className="flex-1" />
                <Input value="••••••••" type="password" disabled className="flex-1" />
                <Button
                  variant="ghost"
                  onClick={() => handleRemoveSecret(secret.id)}
                  className="shrink-0"
                >
                  <IconTrash className="h-4 w-4 mr-2" />
                  Delete
                </Button>
              </div>
            ))}
          </div>
          <div className="flex gap-2">
            <Input
              placeholder="SECRET_KEY"
              value={newSecretKey}
              onChange={(e) => setNewSecretKey(e.target.value)}
              className="flex-1"
            />
            <Input
              placeholder="secret value"
              type="password"
              value={newSecretValue}
              onChange={(e) => setNewSecretValue(e.target.value)}
              className="flex-1"
            />
            <Button onClick={handleAddSecret} className="shrink-0">
              <IconPlus className="h-4 w-4 mr-2" />
              Add
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Installed Agents</CardTitle>
          <CardDescription>Select agents available in this environment</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {SETTINGS_DATA.agents.map((agent) => (
              <label
                key={agent.id}
                className="flex items-center gap-3 p-3 border rounded-lg cursor-pointer hover:bg-accent"
              >
                <input
                  type="checkbox"
                  checked={(environment.installedAgents || []).includes(agent.agent)}
                  onChange={() => handleToggleAgent(agent.agent)}
                  className="h-4 w-4"
                />
                <div className="flex-1">
                  <div className="font-medium">{agent.name}</div>
                  <div className="text-sm text-muted-foreground">{agent.model}</div>
                </div>
              </label>
            ))}
          </div>
        </CardContent>
      </Card>

      <Separator />

      {/* Danger Zone */}
      <Card className="border-destructive">
        <CardHeader>
          <CardTitle className="text-destructive">Danger Zone</CardTitle>
          <CardDescription>
            Irreversible actions that will permanently delete this environment
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div>
              <p className="font-medium">Delete this environment</p>
              <p className="text-sm text-muted-foreground">
                Once deleted, all configuration will be permanently removed
              </p>
            </div>
            <Button variant="destructive" onClick={() => setDeleteDialogOpen(true)}>
              <IconTrash className="h-4 w-4 mr-2" />
              Delete Environment
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Environment</DialogTitle>
            <DialogDescription>
              This action cannot be undone. This will permanently delete the environment &quot;
              {environment.name}&quot; and all its data.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <p className="text-sm">
              Please type <span className="font-mono font-bold">delete</span> to confirm:
            </p>
            <Input
              value={deleteConfirmText}
              onChange={(e) => setDeleteConfirmText(e.target.value)}
              placeholder="delete"
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteEnvironment}
              disabled={deleteConfirmText !== 'delete'}
            >
              Delete Environment
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
