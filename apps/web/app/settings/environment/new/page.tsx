'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { IconCube } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@kandev/ui/card';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import { Separator } from '@kandev/ui/separator';
import { Textarea } from '@kandev/ui/textarea';
import type { AgentType, BaseDocker } from '@/lib/settings/types';
import { createEnvironmentAction } from '@/app/actions/environments';
import { getWebSocketClient } from '@/lib/ws/connection';

const AGENT_OPTIONS: Array<{ id: AgentType; label: string; description: string }> = [
  { id: 'claude-code', label: 'Claude Code', description: 'Full-featured coding agent.' },
  { id: 'codex', label: 'Codex', description: 'Fast iterative coding agent.' },
  { id: 'auggie', label: 'Augment', description: 'Code review + planning agent.' },
];

const BASE_IMAGE_LABELS: Record<BaseDocker, string> = {
  universal: 'Universal (Ubuntu)',
  golang: 'Golang',
  node: 'Node.js',
  python: 'Python',
};

function generateDockerfile(baseImage: BaseDocker, agents: AgentType[]) {
  const baseImageMap: Record<BaseDocker, string> = {
    universal: 'ubuntu:22.04',
    golang: 'golang:1.23-alpine',
    node: 'node:20-bullseye',
    python: 'python:3.12-slim',
  };

  const agentList = agents.length ? agents.join(', ') : 'none';

  return [
    `FROM ${baseImageMap[baseImage]}`,
    '',
    'RUN apt-get update && apt-get install -y curl git jq && rm -rf /var/lib/apt/lists/*',
    '',
    `# Install agents: ${agentList}`,
    '# TODO: install selected agents here',
    '',
    'WORKDIR /workspace',
    '',
    '# Repository will be mounted at runtime',
  ].join('\n');
}

function EnvironmentDetailsSection({
  name,
  onNameChange,
  baseImage,
  onBaseImageChange,
  imageTag,
  onImageTagChange,
}: {
  name: string;
  onNameChange: (value: string) => void;
  baseImage: BaseDocker;
  onBaseImageChange: (value: BaseDocker) => void;
  imageTag: string;
  onImageTagChange: (value: string) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Environment Details</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="env-name">Environment name</Label>
            <Input id="env-name" value={name} onChange={(event) => onNameChange(event.target.value)} />
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>Base image</Label>
            <Select value={baseImage} onValueChange={(value) => onBaseImageChange(value as BaseDocker)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(BASE_IMAGE_LABELS).map(([key, label]) => (
                  <SelectItem key={key} value={key}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="image-tag">Image tag</Label>
            <Input id="image-tag" value={imageTag} onChange={(event) => onImageTagChange(event.target.value)} />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

type ImageBuildCardProps = {
  installAgents: AgentType[];
  onToggleAgent: (agentId: AgentType) => void;
  onBuildImage: () => void;
  showDockerfileEditor: boolean;
  dockerfile: string;
  onDockerfileChange: (value: string) => void;
};

function ImageBuildCard({
  installAgents, onToggleAgent, onBuildImage,
  showDockerfileEditor, dockerfile, onDockerfileChange,
}: ImageBuildCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <IconCube className="h-4 w-4" />
          Image Build
        </CardTitle>
        <CardDescription>Select agents to install in the image.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-2 sm:grid-cols-3">
          {AGENT_OPTIONS.map((agent) => (
            <label key={agent.id} className="flex items-start gap-2 rounded-md border border-border/70 p-2 text-sm">
              <input type="checkbox" className="mt-0.5" checked={installAgents.includes(agent.id)} onChange={() => onToggleAgent(agent.id)} />
              <span>
                <span className="font-medium block">{agent.label}</span>
                <span className="text-xs text-muted-foreground">{agent.description}</span>
              </span>
            </label>
          ))}
        </div>
        <p className="text-xs text-muted-foreground">Agent folders will be mounted as volumes at runtime.</p>
        <div className="flex items-center justify-between flex-wrap gap-2">
          <p className="text-xs text-muted-foreground">Repositories are mounted into the container at runtime.</p>
          <Button type="button" onClick={onBuildImage}>Build image</Button>
        </div>
        {showDockerfileEditor && (
          <div className="space-y-2">
            <Label htmlFor="dockerfile">Dockerfile</Label>
            <Textarea id="dockerfile" value={dockerfile} onChange={(event) => onDockerfileChange(event.target.value)} rows={12} className="font-mono text-sm" />
            <p className="text-xs text-muted-foreground">Edit the Dockerfile before building locally.</p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export default function EnvironmentCreatePage() {
  const router = useRouter();
  const [name, setName] = useState('Custom Image');
  const [baseImage, setBaseImage] = useState<BaseDocker>('universal');
  const [imageTag, setImageTag] = useState('kandev/custom:latest');
  const [installAgents, setInstallAgents] = useState<AgentType[]>([]);
  const [dockerfile, setDockerfile] = useState('');
  const [showDockerfileEditor, setShowDockerfileEditor] = useState(false);
  const [isCreating, setIsCreating] = useState(false);

  const handleToggleAgent = (agentId: AgentType) => {
    setInstallAgents((prev) => prev.includes(agentId) ? prev.filter((id) => id !== agentId) : [...prev, agentId]);
  };

  const handleBuildImage = () => {
    setDockerfile(generateDockerfile(baseImage, installAgents));
    setShowDockerfileEditor(true);
  };

  const handleCreate = async () => {
    setIsCreating(true);
    try {
      const payload = { name, kind: 'docker_image', image_tag: imageTag, dockerfile, build_config: { base_image: baseImage, install_agents: installAgents.join(',') } };
      const client = getWebSocketClient();
      if (client) { await client.request('environment.create', payload); }
      else { await createEnvironmentAction(payload); }
      router.push('/settings/environments');
    } finally { setIsCreating(false); }
  };

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Create Custom Environment</h2>
        <p className="text-sm text-muted-foreground mt-1">Build a Docker image tailored to your workflow.</p>
      </div>
      <Separator />
      <EnvironmentDetailsSection name={name} onNameChange={setName} baseImage={baseImage} onBaseImageChange={setBaseImage} imageTag={imageTag} onImageTagChange={setImageTag} />
      <ImageBuildCard
        installAgents={installAgents} onToggleAgent={handleToggleAgent} onBuildImage={handleBuildImage}
        showDockerfileEditor={showDockerfileEditor} dockerfile={dockerfile} onDockerfileChange={setDockerfile}
      />
      <div className="flex items-center justify-end gap-2">
        <Button variant="outline" onClick={() => router.push('/settings/environments')}>Cancel</Button>
        <Button onClick={handleCreate} disabled={isCreating}>{isCreating ? 'Creating...' : 'Create Environment'}</Button>
      </div>
    </div>
  );
}
