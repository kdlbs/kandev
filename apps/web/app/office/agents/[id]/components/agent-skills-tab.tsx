"use client";

import { useEffect, useMemo, useState, useCallback } from "react";
import Link from "@/components/routing/app-link";
import { toast } from "@/lib/toast/sonner";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { listSkills, updateAgentProfile } from "@/lib/api/domains/office-api";
import type { AgentProfile, Skill } from "@/lib/state/slices/office/types";
import { useTranslation } from "react-i18next";

type AgentSkillsTabProps = {
  agent: AgentProfile;
};

/**
 * Hydrate the office skills store on mount. The workspace Skills page
 * populates it as a side effect of viewing, but a user landing
 * directly on /office/agents/<id>/skills wouldn't have run that path
 * yet. Hitting listSkills also triggers the backend's lazy per-
 * workspace system-skill sync, so a fresh workspace shows the
 * bundled set on first visit.
 */
function useHydrateSkills() {
  const setSkills = useAppStore((s) => s.setSkills);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    listSkills(workspaceId)
      .then((res) => {
        if (!cancelled) setSkills(res.skills ?? []);
      })
      .catch(() => {
        // Non-fatal: existing store contents (possibly empty) render
        // the "No skills registered" CTA, which still lets the user
        // pivot to the Skills page.
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, setSkills]);
}

/**
 * `agent.desiredSkills` (slugs) and `agent.skillIds` (ids) are independent
 * columns that the runtime unions at launch. Agents seeded via office config
 * import, or from before the ids column existed, can have skills recorded
 * only as slugs. Resolve those slugs against the loaded skill catalog so the
 * UI shows the same picture the runtime already acts on.
 */
function unionSkillIds(agent: AgentProfile, skills: Skill[]): string[] {
  const ids = new Set(agent.skillIds ?? []);
  if (agent.desiredSkills?.length) {
    const bySlug = new Map(skills.map((skill) => [skill.slug, skill.id]));
    for (const slug of agent.desiredSkills) {
      const id = bySlug.get(slug);
      if (id) ids.add(id);
    }
  }
  return Array.from(ids);
}

type SkillRowProps = {
  skill: Skill;
  agentRole?: string;
  isSelected: boolean;
  onToggle: (id: string) => void;
};

function SkillRow({ skill, agentRole, isSelected, onToggle }: SkillRowProps) {
  const { t } = useTranslation();
  const isDefault = skill.isSystem && (skill.defaultForRoles ?? []).includes(agentRole ?? "");
  return (
    <label
      data-testid={`skill-toggle-${skill.slug}`}
      className="flex items-center gap-3 py-1.5 px-2 rounded-md hover:bg-accent/50 cursor-pointer"
    >
      <Checkbox
        checked={isSelected}
        onCheckedChange={() => onToggle(skill.id)}
        className="cursor-pointer"
        data-testid={`skill-toggle-checkbox-${skill.slug}`}
      />
      <span className="text-sm">{skill.name}</span>
      {skill.isSystem && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Badge variant="outline" className="text-[10px] text-muted-foreground">
              {t("office:system")}
            </Badge>
          </TooltipTrigger>
          <TooltipContent>
            {t("office:bundledWithKandev")}
            {skill.systemVersion ? ` v${skill.systemVersion}` : ""}
          </TooltipContent>
        </Tooltip>
      )}
      {isDefault && (
        <Tooltip>
          <TooltipTrigger asChild>
            {/* `agentRole` is the wire role id, interpolated as data. */}
            <span className="text-[10px] text-muted-foreground">
              {t("office:defaultForRole", { role: agentRole })}
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {t("office:skillAutoAttachedToRole", { role: agentRole })}
          </TooltipContent>
        </Tooltip>
      )}
      <span className="text-xs text-muted-foreground ml-auto">{skill.slug}</span>
    </label>
  );
}

export function AgentSkillsTab({ agent }: AgentSkillsTabProps) {
  const { t } = useTranslation();
  useHydrateSkills();
  const skills = useAppStore((s) => s.office.skills);
  const updateStore = useAppStore((s) => s.updateOfficeAgentProfile);
  const initialSkillIds = useMemo(() => unionSkillIds(agent, skills), [agent, skills]);
  const [skillIds, setSkillIds] = useState<string[]>(initialSkillIds);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  // `skills` hydrates asynchronously after mount (see useHydrateSkills), so
  // the union above is re-derived once the catalog arrives. Once the user
  // starts editing, further catalog refreshes must not clobber their picks.
  useEffect(() => {
    if (!dirty) setSkillIds(initialSkillIds);
  }, [initialSkillIds, dirty]);

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
      // Slugs already on the agent that don't resolve against this
      // workspace's catalog (e.g. attached cross-workspace, see
      // GetSkillFromConfig's fallback) are still live attachments at
      // launch. Only drop a slug the user actually unticked here.
      const catalogSlugs = new Set(skills.map((skill) => skill.slug));
      const unresolved = (agent.desiredSkills ?? []).filter((slug) => !catalogSlugs.has(slug));
      const selectedSlugs = skills
        .filter((skill) => skillIds.includes(skill.id))
        .map((skill) => skill.slug);
      const desiredSkills = [...unresolved, ...selectedSlugs];
      await updateAgentProfile(agent.id, { skillIds, desiredSkills });
      updateStore(agent.workspaceId, agent.id, { skillIds, desiredSkills });
      setDirty(false);
      toast.success(t("office:skillsUpdated"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToUpdateSkills"));
    } finally {
      setSaving(false);
    }
  }, [agent.id, agent.desiredSkills, agent.workspaceId, skillIds, skills, updateStore, t]);

  if (skills.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 gap-3">
        <p className="text-sm text-muted-foreground">{t("office:noSkillsRegisteredYet")}</p>
        <Button asChild variant="outline" size="sm" className="cursor-pointer">
          <Link href="/office/workspace/skills">{t("office:manageSkillsInCompany")}</Link>
        </Button>
      </div>
    );
  }

  const selected = new Set(skillIds);

  return (
    <div className="space-y-4 mt-4">
      <p className="text-xs text-muted-foreground">{t("office:skillsThisAgentOwnsSkillsAre")}</p>
      <div className="space-y-1.5">
        {skills.map((skill) => (
          <SkillRow
            key={skill.id}
            skill={skill}
            agentRole={agent.role}
            isSelected={selected.has(skill.id)}
            onToggle={toggle}
          />
        ))}
      </div>
      {dirty && (
        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={saving} className="cursor-pointer">
            {saving ? t("office:saving") : t("office:saveSkills")}
          </Button>
        </div>
      )}
    </div>
  );
}
