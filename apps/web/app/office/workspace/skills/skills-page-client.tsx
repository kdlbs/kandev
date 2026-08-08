"use client";

import { useCallback, useEffect, useState } from "react";
import { IconBoxMultiple } from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
import { useAppStore } from "@/components/state-provider";
import * as officeApi from "@/lib/api/domains/office-api";
import type { Skill } from "@/lib/state/slices/office/types";
import { SkillList } from "./skill-list";
import { SkillDetail } from "./skill-detail";
import { CreateSkillForm } from "./create-skill-form";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
  const addSkill = useAppStore((s) => s.addSkill);
  const updateSkillInStore = useAppStore((s) => s.updateSkill);
  const removeSkillFromStore = useAppStore((s) => s.removeSkill);

  const handleCreate = useCallback(
    async (data: Partial<Skill>) => {
      if (!activeWorkspaceId) return;
      try {
        const res = await officeApi.createSkill(activeWorkspaceId, data);
        addSkill(res.skill);
        setSelectedId(res.skill.id);
        setViewMode("view");
      } catch (err) {
        const msg = err instanceof Error ? err.message : "";
        // Matched against the BACKEND's English error text — protocol, not copy.
        if (msg.includes("already exists") || msg.includes("duplicate") || msg.includes("unique")) {
          toast.error(t("office:aSkillWithThisNameAlready"));
        } else {
          toast.error(t("office:failedToCreateSkill"));
        }
      }
    },
    [activeWorkspaceId, addSkill, setSelectedId, setViewMode],
  );

  const handleSave = useCallback(
    async (id: string, patch: Partial<Skill>) => {
      await officeApi.updateSkill(id, patch);
      updateSkillInStore(id, patch);
    },
    [updateSkillInStore],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      await officeApi.deleteSkill(id);
      removeSkillFromStore(id);
      if (selectedId === id) {
        const remaining = skills.filter((s) => s.id !== id);
        setSelectedId(remaining[0]?.id ?? null);
      }
    },
    [removeSkillFromStore, selectedId, skills, setSelectedId],
  );

  const handleImport = useCallback(
    async (source: string) => {
      if (!activeWorkspaceId) return;
      const res = await officeApi.importSkill(activeWorkspaceId, source);
      for (const skill of res.skills) {
        addSkill(skill);
      }
      if (res.skills.length > 0) {
        setSelectedId(res.skills[0].id);
        setViewMode("view");
      }
    },
    [activeWorkspaceId, addSkill, setSelectedId, setViewMode],
  );

  return { handleCreate, handleSave, handleDelete, handleImport };
}

export function SkillsPageClient({ initialSkills }: SkillsPageClientProps) {
  const skills = useAppStore((s) => s.office.skills);
  const setSkills = useAppStore((s) => s.setSkills);
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<ViewMode>("view");

  useEffect(() => {
    if (initialSkills.length > 0) {
      setSkills(initialSkills);
    }
  }, [initialSkills, setSkills]);

  const fetchSkills = useCallback(() => {
    if (!activeWorkspaceId) return;
    officeApi
      .listSkills(activeWorkspaceId)
      .then((res) => {
        setSkills(res.skills ?? []);
      })
      .catch(() => {});
  }, [activeWorkspaceId, setSkills]);

  useEffect(() => {
    fetchSkills();
  }, [fetchSkills]);

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
        onRefresh={fetchSkills}
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
  const { t } = useTranslation();
  if (viewMode === "create") {
    return <CreateSkillForm onCreate={onCreate} onCancel={onCancelCreate} />;
  }
  if (selectedSkill) {
    return <SkillDetail skill={selectedSkill} onSave={onSave} onDelete={onDelete} />;
  }
  return (
    <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
      <IconBoxMultiple className="h-12 w-12 mb-4 opacity-30" />
      <p className="text-sm">{t("office:selectASkillToView")}</p>
      <p className="text-xs mt-1">{t("office:skillsTeachAgentsHowToPerform")}</p>
    </div>
  );
}
