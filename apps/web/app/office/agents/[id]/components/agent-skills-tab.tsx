"use client";

import { useState, useCallback } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { updateAgentProfile } from "@/lib/api/domains/office-api";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import { qk } from "@/lib/query/keys";
import type { AgentProfile } from "@/lib/state/slices/office/types";

type AgentSkillsTabProps = {
  agent: AgentProfile;
};

export function AgentSkillsTab({ agent }: AgentSkillsTabProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const qc = useQueryClient();
  // TQ query handles the hydration that useHydrateSkills previously did
  // imperatively. It also triggers the backend's lazy system-skill sync
  // on first view of /office/agents/<id>/skills.
  const { data: skills = [] } = useQuery({
    ...officeQueryOptions.skills(workspaceId ?? ""),
    enabled: !!workspaceId,
  });
  const [skillIds, setSkillIds] = useState<string[]>(agent.skillIds ?? []);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  const toggle = useCallback((id: string) => {
    setSkillIds((prev) => {
      const next = prev.includes(id) ? prev.filter((s) => s !== id) : [...prev, id];
      setDirty(true);
      return next;
    });
  }, []);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      await updateAgentProfile(agent.id, { skillIds });
      if (workspaceId) void qc.invalidateQueries({ queryKey: qk.office.agents(workspaceId) });
      setDirty(false);
      toast.success("Skills updated");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update skills");
    } finally {
      setSaving(false);
    }
  }, [agent.id, skillIds, workspaceId, qc]);

  if (skills.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 gap-3">
        <p className="text-sm text-muted-foreground">No skills registered yet.</p>
        <Button asChild variant="outline" size="sm" className="cursor-pointer">
          <Link href="/office/workspace/skills">Manage skills in Company</Link>
        </Button>
      </div>
    );
  }

  const selected = new Set(skillIds);

  return (
    <div className="space-y-4 mt-4">
      <p className="text-xs text-muted-foreground">
        Skills this agent owns. Skills are injected into the agent&apos;s system prompt at session
        start.
      </p>
      <SkillList skills={skills} selected={selected} agentRole={agent.role} onToggle={toggle} />
      {dirty && (
        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={saving} className="cursor-pointer">
            {saving ? "Saving..." : "Save skills"}
          </Button>
        </div>
      )}
    </div>
  );
}

type Skill = {
  id: string;
  name: string;
  slug: string;
  isSystem?: boolean;
  systemVersion?: string;
  defaultForRoles?: string[];
};

function SkillList({
  skills,
  selected,
  agentRole,
  onToggle,
}: {
  skills: Skill[];
  selected: Set<string>;
  agentRole: string;
  onToggle: (id: string) => void;
}) {
  return (
    <div className="space-y-1.5">
      {skills.map((skill) => {
        const isDefault = skill.isSystem && (skill.defaultForRoles ?? []).includes(agentRole);
        return (
          <label
            key={skill.id}
            data-testid={`skill-toggle-${skill.slug}`}
            className="flex items-center gap-3 py-1.5 px-2 rounded-md hover:bg-accent/50 cursor-pointer"
          >
            <Checkbox
              checked={selected.has(skill.id)}
              onCheckedChange={() => onToggle(skill.id)}
              className="cursor-pointer"
              data-testid={`skill-toggle-checkbox-${skill.slug}`}
            />
            <span className="text-sm">{skill.name}</span>
            {skill.isSystem && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Badge variant="outline" className="text-[10px] text-muted-foreground">
                    System
                  </Badge>
                </TooltipTrigger>
                <TooltipContent>
                  Bundled with kandev{skill.systemVersion ? ` v${skill.systemVersion}` : ""}
                </TooltipContent>
              </Tooltip>
            )}
            {isDefault && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="text-[10px] text-muted-foreground">default for {agentRole}</span>
                </TooltipTrigger>
                <TooltipContent>
                  This skill is auto-attached to new {agentRole} agents. You can still untick it for
                  this agent.
                </TooltipContent>
              </Tooltip>
            )}
            <span className="text-xs text-muted-foreground ml-auto">{skill.slug}</span>
          </label>
        );
      })}
    </div>
  );
}
