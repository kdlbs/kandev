'use client';

import { use, useState } from 'react';
import { useRouter } from 'next/navigation';
import { IconTrash } from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { Switch } from '@/components/ui/switch';
import { Slider } from '@/components/ui/slider';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { SETTINGS_DATA } from '@/lib/settings/dummy-data';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import type { AgentProfile, AgentType } from '@/lib/settings/types';

const AGENT_LABELS: Record<AgentType, string> = {
  'claude-code': 'Claude Code',
  'codex': 'Codex',
  'auggie': 'Auggie',
};

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

export default function AgentEditPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const [agent, setAgent] = useState<AgentProfile | undefined>(
    SETTINGS_DATA.agents.find((a) => a.id === id)
  );
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');

  if (!agent) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Agent not found</p>
            <Button className="mt-4" onClick={() => router.push('/settings/agents')}>
              Go to Agents
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const handleDeleteAgent = () => {
    if (deleteConfirmText === 'delete') {
      router.push('/settings/agents');
      setDeleteDialogOpen(false);
    }
  };

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">{agent.name}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Configure agent behavior and capabilities
        </p>
      </div>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle>Agent Name</CardTitle>
        </CardHeader>
        <CardContent>
          <Input
            value={agent.name}
            onChange={(e) => setAgent({ ...agent, name: e.target.value })}
            placeholder="Code Assistant"
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Agent Configuration</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>Agent Type</Label>
              <Select
                value={agent.agent}
                onValueChange={(value) => {
                  const agentType = value as AgentType;
                  setAgent({
                    ...agent,
                    agent: agentType,
                    model: MODEL_OPTIONS[agentType][0].value
                  });
                }}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="claude-code">Claude Code</SelectItem>
                  <SelectItem value="codex">Codex</SelectItem>
                  <SelectItem value="auggie">Auggie</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Model</Label>
              <Select
                value={agent.model}
                onValueChange={(value) => setAgent({ ...agent, model: value })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {MODEL_OPTIONS[agent.agent].map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Advanced Settings</CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label>Temperature</Label>
                <p className="text-sm text-muted-foreground">
                  Controls randomness in responses (0 = deterministic, 1 = creative)
                </p>
              </div>
              <span className="text-sm font-medium">{agent.temperature.toFixed(1)}</span>
            </div>
            <Slider
              value={[agent.temperature]}
              onValueChange={([value]) => setAgent({ ...agent, temperature: value })}
              min={0}
              max={1}
              step={0.1}
              className="w-full"
            />
          </div>

          <Separator />

          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label>Auto-approve Actions</Label>
              <p className="text-sm text-muted-foreground">
                Automatically execute tool calls without confirmation
              </p>
            </div>
            <Switch
              checked={agent.autoApprove}
              onCheckedChange={(checked) => setAgent({ ...agent, autoApprove: checked })}
            />
          </div>
        </CardContent>
      </Card>

      <Separator />

      {/* Danger Zone */}
      <Card className="border-destructive">
        <CardHeader>
          <CardTitle className="text-destructive">Danger Zone</CardTitle>
          <CardDescription>
            Irreversible actions that will permanently delete this agent
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div>
              <p className="font-medium">Delete this agent</p>
              <p className="text-sm text-muted-foreground">
                Once deleted, all configuration will be permanently removed
              </p>
            </div>
            <Button variant="destructive" onClick={() => setDeleteDialogOpen(true)}>
              <IconTrash className="h-4 w-4 mr-2" />
              Delete Agent
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Agent</DialogTitle>
            <DialogDescription>
              This action cannot be undone. This will permanently delete the agent &quot;
              {agent.name}&quot; and all its data.
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
              onClick={handleDeleteAgent}
              disabled={deleteConfirmText !== 'delete'}
            >
              Delete Agent
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
