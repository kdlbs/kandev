'use client';

import { useState } from 'react';
import { IconPlus } from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Card, CardContent } from '@/components/ui/card';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { EnvironmentCard } from '@/components/settings/environment-card';
import { KeyValueInput } from '@/components/settings/key-value-input';
import { SETTINGS_DATA } from '@/lib/settings/dummy-data';
import type { Environment, EnvironmentType, BaseDocker, KeyValue, AgentType } from '@/lib/settings/types';
import { generateUUID } from '@/lib/utils';

export default function EnvironmentsSettingsPage() {
  const [environments, setEnvironments] = useState<Environment[]>(SETTINGS_DATA.environments);
  const [isAdding, setIsAdding] = useState(false);
  const [newEnv, setNewEnv] = useState<{
    name: string;
    type: EnvironmentType;
    baseDocker: BaseDocker;
    envVariables: KeyValue[];
    secrets: KeyValue[];
    setupScript: string;
    installedAgents: AgentType[];
  }>({
    name: '',
    type: 'local-docker',
    baseDocker: 'universal',
    envVariables: [],
    secrets: [],
    setupScript: '',
    installedAgents: [],
  });

  const handleAdd = (e: React.FormEvent) => {
    e.preventDefault();
    const environment: Environment = {
      id: generateUUID(),
      ...newEnv,
    };
    setEnvironments([...environments, environment]);
    setIsAdding(false);
    setNewEnv({
      name: '',
      type: 'local-docker',
      baseDocker: 'universal',
      envVariables: [],
      secrets: [],
      setupScript: '',
      installedAgents: [],
    });
  };

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-2xl font-bold">Environments</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Configure execution environments for your agents
          </p>
        </div>
        <Button size="sm" onClick={() => setIsAdding(true)}>
          <IconPlus className="h-4 w-4 mr-2" />
          Add Environment
        </Button>
      </div>

      <Separator />

      <div className="space-y-4">
        <div className="grid gap-3">
          {isAdding && (
            <Card>
              <CardContent className="pt-6">
                <form onSubmit={handleAdd} className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="env-name">Environment Name</Label>
                    <Input
                      id="env-name"
                      value={newEnv.name}
                      onChange={(e) => setNewEnv({ ...newEnv, name: e.target.value })}
                      placeholder="Development"
                      required
                    />
                  </div>

                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label htmlFor="env-type">Environment Type</Label>
                      <Select
                        value={newEnv.type}
                        onValueChange={(value) =>
                          setNewEnv({ ...newEnv, type: value as EnvironmentType })
                        }
                      >
                        <SelectTrigger id="env-type">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="local-docker">Local Docker</SelectItem>
                          <SelectItem value="remote-docker">Remote Docker</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="base-docker">Base Docker Image</Label>
                      <Select
                        value={newEnv.baseDocker}
                        onValueChange={(value) =>
                          setNewEnv({ ...newEnv, baseDocker: value as BaseDocker })
                        }
                      >
                        <SelectTrigger id="base-docker">
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
                    <Label>Environment Variables</Label>
                    <KeyValueInput
                      items={newEnv.envVariables}
                      onChange={(items) => setNewEnv({ ...newEnv, envVariables: items })}
                      keyPlaceholder="Variable name"
                      valuePlaceholder="Value"
                      addButtonLabel="Add Variable"
                    />
                  </div>

                  <div className="space-y-2">
                    <Label>Secrets</Label>
                    <KeyValueInput
                      items={newEnv.secrets}
                      onChange={(items) => setNewEnv({ ...newEnv, secrets: items })}
                      keyPlaceholder="Secret name"
                      valuePlaceholder="Secret value"
                      addButtonLabel="Add Secret"
                      masked
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="setup-script">Setup Script</Label>
                    <Textarea
                      id="setup-script"
                      value={newEnv.setupScript}
                      onChange={(e) =>
                        setNewEnv({ ...newEnv, setupScript: e.target.value })
                      }
                      placeholder="#!/bin/bash&#10;apt-get update&#10;apt-get install -y git"
                      rows={5}
                      className="font-mono text-sm"
                    />
                  </div>

                  <div className="flex gap-2 justify-end">
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setIsAdding(false)}
                    >
                      Cancel
                    </Button>
                    <Button type="submit">Add Environment</Button>
                  </div>
                </form>
              </CardContent>
            </Card>
          )}

          {environments.map((env) => (
            <EnvironmentCard
              key={env.id}
              environment={env}
            />
          ))}

          {environments.length === 0 && !isAdding && (
            <Card>
              <CardContent className="py-8 text-center">
                <p className="text-sm text-muted-foreground">
                  No environments configured. Add your first environment to get started.
                </p>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
