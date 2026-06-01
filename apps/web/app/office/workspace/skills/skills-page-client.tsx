"use client";

import { useCallback, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { IconBoxMultiple } from "@tabler/icons-react";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import * as officeApi from "@/lib/api/domains/office-api";
import * as skillsApi from "@/lib/api/domains/office-skills-api";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import type { Skill } from "@/lib/state/slices/office/types";
import { SkillList } from "./skill-list";
import { SkillDetail } from "./skill-detail";
import { CreateSkillForm } from "./create-skill-form";

type ViewMode = "view" | "create";

function useSkillActions(
  activeWorkspaceId: string | null,
  selectedId: string | null,
  setSelectedId: (id: string | null) => void,
  setViewMode: (mode: ViewMode) => void,
  skills: Skill[],
) {
  const qc = useQueryClient();

  function invalidate() {
    if (activeWorkspaceId) {
      void qc.invalidateQueries({
        queryKey: ["office", activeWorkspaceId, "skills"],
      });
    }
  }

  const handleCreate = useCallback(
    async (data: Partial<Skill>) => {
      if (!activeWorkspaceId) return;
      try {
        const res = await skillsApi.createSkill(activeWorkspaceId, data);
        invalidate();
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [activeWorkspaceId, qc, setSelectedId, setViewMode],
  );

  const handleSave = useCallback(
    async (id: string, patch: Partial<Skill>) => {
      await skillsApi.updateSkill(id, patch);
      invalidate();
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [activeWorkspaceId, qc],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      await skillsApi.deleteSkill(id);
      invalidate();
      if (selectedId === id) {
        const remaining = skills.filter((s) => s.id !== id);
        setSelectedId(remaining[0]?.id ?? null);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [activeWorkspaceId, qc, selectedId, skills, setSelectedId],
  );

  const handleImport = useCallback(
    async (source: string) => {
      if (!activeWorkspaceId) return;
      const res = await officeApi.importSkill(activeWorkspaceId, source);
      invalidate();
      if (res.skills.length > 0) {
        setSelectedId(res.skills[0].id);
        setViewMode("view");
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [activeWorkspaceId, qc, setSelectedId, setViewMode],
  );

  return { handleCreate, handleSave, handleDelete, handleImport };
}

export function SkillsPageClient() {
  const qc = useQueryClient();
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data: skills = [] } = useQuery({
    ...officeQueryOptions.skills(activeWorkspaceId ?? ""),
    enabled: !!activeWorkspaceId,
  });

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<ViewMode>("view");

  const selectedSkill = skills.find((s) => s.id === selectedId) ?? null;
  const { handleCreate, handleSave, handleDelete, handleImport } = useSkillActions(
    activeWorkspaceId,
    selectedId,
    setSelectedId,
    setViewMode,
    skills,
  );

  function handleRefresh() {
    if (activeWorkspaceId) {
      void qc.invalidateQueries({ queryKey: ["office", activeWorkspaceId, "skills"] });
    }
  }

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
        onRefresh={handleRefresh}
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
