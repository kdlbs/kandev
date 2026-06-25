"use client";

import { useCallback, useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { IconBoxMultiple } from "@tabler/icons-react";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import * as officeApi from "@/lib/api/domains/office-api";
import { qk } from "@/lib/query/keys";
import { officeSkillsQueryOptions } from "@/lib/query/query-options/office";
import type { Skill } from "@/lib/state/slices/office/types";
import { SkillList } from "./skill-list";
import { SkillDetail } from "./skill-detail";
import { CreateSkillForm } from "./create-skill-form";

type ViewMode = "view" | "create";

type SkillsPageClientProps = {
  initialSkills: Skill[];
};

function useSkillActions(
  activeWorkspaceId: string | null,
  selectedId: string | null,
  setSelectedId: (id: string | null) => void,
  setViewMode: (mode: ViewMode) => void,
  skills: Skill[],
) {
  const queryClient = useQueryClient();
  const addSkill = useAppStore((s) => s.addSkill);
  const updateSkillInStore = useAppStore((s) => s.updateSkill);
  const removeSkillFromStore = useAppStore((s) => s.removeSkill);

  const handleCreate = useCallback(
    async (data: Partial<Skill>) => {
      if (!activeWorkspaceId) return;
      try {
        const res = await officeApi.createSkill(activeWorkspaceId, data);
        addSkill(res.skill);
        appendSkill(queryClient, activeWorkspaceId, res.skill);
        setSelectedId(res.skill.id);
        setViewMode("view");
      } catch (err) {
        const msg = err instanceof Error ? err.message : "";
        if (msg.includes("already exists") || msg.includes("duplicate") || msg.includes("unique")) {
          toast.error("A skill with this name already exists");
        } else {
          toast.error("Failed to create skill");
        }
      }
    },
    [activeWorkspaceId, addSkill, queryClient, setSelectedId, setViewMode],
  );

  const handleSave = useCallback(
    async (id: string, patch: Partial<Skill>) => {
      await officeApi.updateSkill(id, patch);
      updateSkillInStore(id, patch);
      if (activeWorkspaceId) {
        patchSkill(queryClient, activeWorkspaceId, id, patch);
      }
    },
    [activeWorkspaceId, queryClient, updateSkillInStore],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      await officeApi.deleteSkill(id);
      removeSkillFromStore(id);
      if (activeWorkspaceId) {
        removeSkill(queryClient, activeWorkspaceId, id);
      }
      if (selectedId === id) {
        const remaining = skills.filter((s) => s.id !== id);
        setSelectedId(remaining[0]?.id ?? null);
      }
    },
    [activeWorkspaceId, queryClient, removeSkillFromStore, selectedId, skills, setSelectedId],
  );

  const handleImport = useCallback(
    async (source: string) => {
      if (!activeWorkspaceId) return;
      const res = await officeApi.importSkill(activeWorkspaceId, source);
      for (const skill of res.skills) {
        addSkill(skill);
        appendSkill(queryClient, activeWorkspaceId, skill);
      }
      if (res.skills.length > 0) {
        setSelectedId(res.skills[0].id);
        setViewMode("view");
      }
    },
    [activeWorkspaceId, addSkill, queryClient, setSelectedId, setViewMode],
  );

  return { handleCreate, handleSave, handleDelete, handleImport };
}

export function SkillsPageClient({ initialSkills }: SkillsPageClientProps) {
  const skillsStore = useAppStore((s) => s.office.skills);
  const setSkills = useAppStore((s) => s.setSkills);
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const queryClient = useQueryClient();
  const skillsQuery = useQuery(officeSkillsQueryOptions(activeWorkspaceId ?? ""));

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<ViewMode>("view");

  useEffect(() => {
    if (initialSkills.length > 0) {
      if (activeWorkspaceId) {
        queryClient.setQueryData(qk.office.skills(activeWorkspaceId), { skills: initialSkills });
      }
      setSkills(initialSkills);
    }
  }, [activeWorkspaceId, initialSkills, queryClient, setSkills]);

  useEffect(() => {
    if (!skillsQuery.data) return;
    setSkills(skillsQuery.data.skills ?? []);
  }, [setSkills, skillsQuery.data]);

  const skills = skillsQuery.data?.skills ?? skillsStore;
  const selectedSkill = skills.find((s) => s.id === selectedId) ?? null;
  const { handleCreate, handleSave, handleDelete, handleImport } = useSkillActions(
    activeWorkspaceId,
    selectedId,
    setSelectedId,
    setViewMode,
    skills,
  );

  return (
    <div className="flex h-full">
      <SkillList
        skills={skills}
        selectedId={selectedId}
        onSelect={(id) => {
          setSelectedId(id);
          setViewMode("view");
        }}
        onAdd={() => {
          setSelectedId(null);
          setViewMode("create");
        }}
        onRefresh={() => void skillsQuery.refetch()}
        onImport={handleImport}
      />
      <div className="flex-1 p-6 overflow-y-auto">
        <SkillContentPanel
          viewMode={viewMode}
          selectedSkill={selectedSkill}
          onCreate={handleCreate}
          onSave={handleSave}
          onDelete={handleDelete}
          onCancelCreate={() => setViewMode("view")}
        />
      </div>
    </div>
  );
}

function appendSkill(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string,
  skill: Skill,
) {
  queryClient.setQueryData<{ skills: Skill[] }>(qk.office.skills(workspaceId), (current) => {
    const skills = current?.skills ?? [];
    if (skills.some((item) => item.id === skill.id)) return { skills };
    return { skills: [...skills, skill] };
  });
}

function patchSkill(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string,
  id: string,
  patch: Partial<Skill>,
) {
  queryClient.setQueryData<{ skills: Skill[] }>(qk.office.skills(workspaceId), (current) => ({
    skills: (current?.skills ?? []).map((skill) =>
      skill.id === id ? { ...skill, ...patch } : skill,
    ),
  }));
}

function removeSkill(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string,
  id: string,
) {
  queryClient.setQueryData<{ skills: Skill[] }>(qk.office.skills(workspaceId), (current) => ({
    skills: (current?.skills ?? []).filter((skill) => skill.id !== id),
  }));
}

function SkillContentPanel({
  viewMode,
  selectedSkill,
  onCreate,
  onSave,
  onDelete,
  onCancelCreate,
}: {
  viewMode: ViewMode;
  selectedSkill: Skill | null;
  onCreate: (data: Partial<Skill>) => void;
  onSave: (id: string, patch: Partial<Skill>) => void;
  onDelete: (id: string) => void;
  onCancelCreate: () => void;
}) {
  if (viewMode === "create") {
    return <CreateSkillForm onCreate={onCreate} onCancel={onCancelCreate} />;
  }
  if (selectedSkill) {
    return <SkillDetail skill={selectedSkill} onSave={onSave} onDelete={onDelete} />;
  }
  return (
    <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
      <IconBoxMultiple className="h-12 w-12 mb-4 opacity-30" />
      <p className="text-sm">Select a skill to view</p>
      <p className="text-xs mt-1">
        Skills teach agents how to perform specific tasks. Import from GitHub or create your own.
      </p>
    </div>
  );
}
