"use client";

import { Checkbox } from "@kandev/ui/checkbox";
import { useAppStore } from "@/components/state-provider";
import type { AgentInstance } from "@/lib/state/slices/orchestrate/types";

type AgentSkillsTabProps = {
  agent: AgentInstance;
};

export function AgentSkillsTab({ agent }: AgentSkillsTabProps) {
  const skills = useAppStore((s) => s.orchestrate.skills);
  const assignedIds = new Set(agent.desiredSkills ?? []);

  if (skills.length === 0) {
    return (
      <div className="flex items-center justify-center py-12">
        <p className="text-sm text-muted-foreground">
          No skills registered yet. Create skills in Company &gt; Skills.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-2 mt-4">
      {skills.map((skill) => (
        <label
          key={skill.id}
          className="flex items-center gap-3 py-1.5 px-2 rounded-md hover:bg-accent/50 cursor-pointer"
        >
          <Checkbox checked={assignedIds.has(skill.id)} disabled className="cursor-pointer" />
          <span className="text-sm">{skill.name}</span>
          <span className="text-xs text-muted-foreground ml-auto">{skill.slug}</span>
        </label>
      ))}
    </div>
  );
}
