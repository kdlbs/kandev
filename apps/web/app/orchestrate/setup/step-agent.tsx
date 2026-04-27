"use client";

import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";

type AgentProfile = {
  id: string;
  label: string;
  agentName: string;
};

type StepAgentProps = {
  agentName: string;
  agentProfileId: string;
  executorPreference: string;
  agentProfiles: AgentProfile[];
  onChange: (patch: {
    agentName?: string;
    agentProfileId?: string;
    executorPreference?: string;
  }) => void;
};

const EXECUTOR_OPTIONS = [
  { value: "local_pc", label: "Local", description: "Runs on your machine as a standalone process" },
  {
    value: "local_docker",
    label: "Docker",
    description: "Runs in a Docker container on your machine",
  },
  { value: "sprites", label: "Sprites", description: "Runs in a remote cloud sandbox" },
] as const;

export function StepAgent({
  agentName,
  agentProfileId,
  executorPreference,
  agentProfiles,
  onChange,
}: StepAgentProps) {
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">Create your CEO agent</h2>
        <p className="text-sm text-muted-foreground mt-1">
          The CEO manages other agents, delegates tasks, and monitors progress.
        </p>
      </div>
      <div className="space-y-4">
        <div>
          <Label htmlFor="agent-name">Agent name</Label>
          <Input
            id="agent-name"
            value={agentName}
            onChange={(e) => onChange({ agentName: e.target.value })}
            placeholder="CEO"
            className="mt-1"
            autoFocus
          />
        </div>
        <ProfileSelector
          value={agentProfileId}
          profiles={agentProfiles}
          onChange={(v) => onChange({ agentProfileId: v })}
        />
        <ExecutorSelector
          value={executorPreference}
          onChange={(v) => onChange({ executorPreference: v })}
        />
      </div>
    </div>
  );
}

function ProfileSelector({
  value,
  profiles,
  onChange,
}: {
  value: string;
  profiles: AgentProfile[];
  onChange: (v: string) => void;
}) {
  return (
    <div>
      <Label>Agent profile</Label>
      <Select value={value || "__none__"} onValueChange={(v) => onChange(v === "__none__" ? "" : v)}>
        <SelectTrigger className="mt-1 cursor-pointer">
          <SelectValue placeholder="Select a profile" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__none__" className="cursor-pointer">
            Default
          </SelectItem>
          {profiles.map((p) => (
            <SelectItem key={p.id} value={p.id} className="cursor-pointer">
              {p.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground mt-1">
        Which AI model and configuration to use for this agent
      </p>
    </div>
  );
}

function ExecutorSelector({
  value,
  onChange,
}: {
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <div>
      <Label>Executor preference</Label>
      <RadioGroup
        value={value || "local_pc"}
        onValueChange={onChange}
        className="mt-2 space-y-2"
      >
        {EXECUTOR_OPTIONS.map((opt) => (
          <label
            key={opt.value}
            className="flex items-start gap-3 rounded-md border p-3 cursor-pointer hover:bg-accent/50 transition-colors"
          >
            <RadioGroupItem value={opt.value} className="mt-0.5" />
            <div>
              <span className="text-sm font-medium">{opt.label}</span>
              <p className="text-xs text-muted-foreground">{opt.description}</p>
            </div>
          </label>
        ))}
      </RadioGroup>
    </div>
  );
}
