'use client';

import { useState } from 'react';
import { IconPlus } from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent } from '@/components/ui/card';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { AgentCard } from '@/components/settings/agent-card';
import { SETTINGS_DATA } from '@/lib/settings/dummy-data';
import type { AgentProfile, AgentType } from '@/lib/settings/types';
import { generateUUID } from '@/lib/utils';

const AGENT_OPTIONS: { value: AgentType; label: string }[] = [
  { value: 'claude-code', label: 'Claude Code' },
  { value: 'codex', label: 'Codex' },
  { value: 'auggie', label: 'Auggie' },
];

const MODEL_OPTIONS: Record<AgentType, { value: string; label: string }[]> = {
  'claude-code': [
    { value: 'claude-sonnet-4.5', label: 'Claude Sonnet 4.5' },
    { value: 'claude-opus-4', label: 'Claude Opus 4' },
    { value: 'claude-haiku-4', label: 'Claude Haiku 4' },
  ],
  'codex': [
    { value: 'codex-v2', label: 'Codex v2' },
    { value: 'codex-v1', label: 'Codex v1' },
  ],
  'auggie': [
    { value: 'auggie-v1', label: 'Auggie v1' },
    { value: 'auggie-lite', label: 'Auggie Lite' },
  ],
};

export default function AgentsSettingsPage() {
  const [agents, setAgents] = useState<AgentProfile[]>(SETTINGS_DATA.agents);
  const [isAdding, setIsAdding] = useState(false);
  const [newAgent, setNewAgent] = useState<{
    agent: AgentType;
    name: string;
    model: string;
    autoApprove: boolean;
    temperature: number;
  }>({
    agent: 'claude-code',
    name: '',
    model: 'claude-sonnet-4.5',
    autoApprove: false,
    temperature: 0.7,
  });

  const handleAdd = (e: React.FormEvent) => {
    e.preventDefault();
    const profile: AgentProfile = {
      id: generateUUID(),
      ...newAgent,
    };
    setAgents([...agents, profile]);
    setIsAdding(false);
    setNewAgent({
      agent: 'claude-code',
      name: '',
      model: 'claude-sonnet-4.5',
      autoApprove: false,
      temperature: 0.7,
    });
  };

  const handleAgentTypeChange = (type: AgentType) => {
    const defaultModel = MODEL_OPTIONS[type][0].value;
    setNewAgent({ ...newAgent, agent: type, model: defaultModel });
  };

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-2xl font-bold">Agents</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Configure agent profiles and their behavior
          </p>
        </div>
        <Button size="sm" onClick={() => setIsAdding(true)}>
          <IconPlus className="h-4 w-4 mr-2" />
          Add Agent Profile
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
                    <Label htmlFor="profile-name">Profile Name</Label>
                    <Input
                      id="profile-name"
                      value={newAgent.name}
                      onChange={(e) => setNewAgent({ ...newAgent, name: e.target.value })}
                      placeholder="My Profile"
                      required
                    />
                  </div>

                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label htmlFor="agent-type">Agent Type</Label>
                      <Select
                        value={newAgent.agent}
                        onValueChange={(value) => handleAgentTypeChange(value as AgentType)}
                      >
                        <SelectTrigger id="agent-type">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {AGENT_OPTIONS.map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="model">Model</Label>
                      <Select
                        value={newAgent.model}
                        onValueChange={(value) => setNewAgent({ ...newAgent, model: value })}
                      >
                        <SelectTrigger id="model">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {MODEL_OPTIONS[newAgent.agent].map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="temperature">Temperature: {newAgent.temperature}</Label>
                    <input
                      id="temperature"
                      type="range"
                      min="0"
                      max="1"
                      step="0.1"
                      value={newAgent.temperature}
                      onChange={(e) =>
                        setNewAgent({ ...newAgent, temperature: parseFloat(e.target.value) })
                      }
                      className="w-full h-2 bg-muted rounded-lg appearance-none cursor-pointer"
                    />
                    <div className="flex justify-between text-xs text-muted-foreground">
                      <span>Precise</span>
                      <span>Creative</span>
                    </div>
                  </div>

                  <div className="flex items-center justify-between p-4 border rounded-md">
                    <div>
                      <Label htmlFor="auto-approve" className="text-base font-medium">
                        Auto-approve all actions
                      </Label>
                      <p className="text-sm text-muted-foreground">
                        Automatically approve agent actions without prompting
                      </p>
                    </div>
                    <button
                      id="auto-approve"
                      type="button"
                      role="switch"
                      aria-checked={newAgent.autoApprove}
                      onClick={() =>
                        setNewAgent({ ...newAgent, autoApprove: !newAgent.autoApprove })
                      }
                      className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                        newAgent.autoApprove ? 'bg-primary' : 'bg-muted'
                      }`}
                    >
                      <span
                        className={`inline-block h-4 w-4 transform rounded-full bg-background transition-transform ${
                          newAgent.autoApprove ? 'translate-x-6' : 'translate-x-1'
                        }`}
                      />
                    </button>
                  </div>

                  <div className="flex gap-2 justify-end">
                    <Button type="button" variant="outline" onClick={() => setIsAdding(false)}>
                      Cancel
                    </Button>
                    <Button type="submit">Add Profile</Button>
                  </div>
                </form>
              </CardContent>
            </Card>
          )}

          {agents.map((agent) => (
            <AgentCard
              key={agent.id}
              agent={agent}
            />
          ))}

          {agents.length === 0 && !isAdding && (
            <Card>
              <CardContent className="py-8 text-center">
                <p className="text-sm text-muted-foreground">
                  No agent profiles configured. Add your first profile to get started.
                </p>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
