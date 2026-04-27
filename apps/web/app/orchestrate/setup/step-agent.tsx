"use client";

import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { useAppStore } from "@/components/state-provider";

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

// Fallback used only when meta has not been hydrated yet (graceful degradation).
const FALLBACK_EXECUTOR_OPTIONS = [
  { id: "local_pc", label: "Local (standalone)", description: "Run on host machine" },
  { id: "local_docker", label: "Local Docker", description: "Run in a local Docker container" },
  {
    id: "sprites",
    label: "Sprites (remote sandbox)",
    description: "Run in a Sprites cloud environment",
  },
];

export function StepAgent({
  agentName,
  agentProfileId,
  executorPreference,
  agentProfiles,
  onChange,
}: StepAgentProps) {
  const meta = useAppStore((s) => s.orchestrate.meta);
  const executorOptions = meta?.executorTypes ?? FALLBACK_EXECUTOR_OPTIONS;

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
          options={executorOptions}
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
      <Select
        value={value || "__none__"}
        onValueChange={(v) => onChange(v === "__none__" ? "" : v)}
      >
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
  options,
  onChange,
}: {
  value: string;
  options: { id: string; label: string; description: string }[];
  onChange: (v: string) => void;
}) {
  return (
    <div>
      <Label>Executor preference</Label>
      <RadioGroup value={value || "local_pc"} onValueChange={onChange} className="mt-2 space-y-2">
        {options.map((opt) => (
          <label
            key={opt.id}
            className="flex items-start gap-3 rounded-md border p-3 cursor-pointer hover:bg-accent/50 transition-colors"
          >
            <RadioGroupItem value={opt.id} className="mt-0.5" />
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
