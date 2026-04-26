"use client";

import { useCallback, useEffect, useState } from "react";
import { IconBoxMultiple } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import * as orchestrateApi from "@/lib/api/domains/orchestrate-api";
import type { Skill, SkillSourceType } from "@/lib/state/slices/orchestrate/types";
import { SkillList } from "./skill-list";
import { SkillEditor } from "./skill-editor";
import { CreateSkillForm } from "./create-skill-form";

type ViewMode = "view" | "create";

export default function SkillsPage() {
  const skills = useAppStore((s) => s.orchestrate.skills);
  const setSkills = useAppStore((s) => s.setSkills);
  const addSkill = useAppStore((s) => s.addSkill);
  const updateSkillInStore = useAppStore((s) => s.updateSkill);
  const removeSkillFromStore = useAppStore((s) => s.removeSkill);
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<ViewMode>("view");

  useEffect(() => {
    if (!activeWorkspaceId) return;
    orchestrateApi
      .listSkills(activeWorkspaceId)
      .then((res) => setSkills(res.skills))
      .catch(() => {});
  }, [activeWorkspaceId, setSkills]);

  const selectedSkill = skills.find((s) => s.id === selectedId) ?? null;

  const handleCreate = useCallback(
    async (data: {
      name: string;
      description: string;
      sourceType: SkillSourceType;
      sourceLocator: string;
      content: string;
    }) => {
      if (!activeWorkspaceId) return;
      const res = await orchestrateApi.createSkill(activeWorkspaceId, {
        name: data.name,
        description: data.description,
        sourceType: data.sourceType,
        sourceLocator: data.sourceLocator,
        content: data.content,
      });
      addSkill(res.skill);
      setSelectedId(res.skill.id);
      setViewMode("view");
    },
    [activeWorkspaceId, addSkill],
  );

  const handleSave = useCallback(
    async (id: string, patch: Partial<Skill>) => {
      await orchestrateApi.updateSkill(id, patch);
      updateSkillInStore(id, patch);
    },
    [updateSkillInStore],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      await orchestrateApi.deleteSkill(id);
      removeSkillFromStore(id);
      if (selectedId === id) {
        setSelectedId(null);
      }
    },
    [removeSkillFromStore, selectedId],
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
      />
      <div className="flex-1 p-6 overflow-y-auto">
        {viewMode === "create" ? (
          <CreateSkillForm onCreate={handleCreate} onCancel={() => setViewMode("view")} />
        ) : selectedSkill ? (
          <SkillEditor skill={selectedSkill} onSave={handleSave} onDelete={handleDelete} />
        ) : (
          <EmptyState />
        )}
      </div>
    </div>
  );
}

function EmptyState() {
  return (
    <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
      <IconBoxMultiple className="h-12 w-12 mb-4 opacity-30" />
      <p className="text-sm">Select a skill to view</p>
    </div>
  );
}
