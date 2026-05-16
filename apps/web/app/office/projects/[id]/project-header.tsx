"use client";

import { useState, useCallback } from "react";
import { IconDeviceFloppy } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { toast } from "sonner";
import { updateProject } from "@/lib/api/domains/office-api";
import { useAppStore } from "@/components/state-provider";
import type { Project, ProjectStatus } from "@/lib/state/slices/office/types";

const STATUS_OPTIONS: { value: ProjectStatus; label: string }[] = [
  { value: "active", label: "Active" },
  { value: "completed", label: "Completed" },
  { value: "on_hold", label: "On Hold" },
  { value: "archived", label: "Archived" },
];

type ProjectHeaderProps = {
  project: Project;
};

export function ProjectHeader({ project }: ProjectHeaderProps) {
  const updateProjectStore = useAppStore((s) => s.updateProject);

  const [name, setName] = useState(project.name);
  const [description, setDescription] = useState(project.description ?? "");
  const [status, setStatus] = useState<ProjectStatus>(project.status);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const patch: Partial<Project> = {};
      if (name !== project.name) patch.name = name;
      if (description !== (project.description ?? "")) patch.description = description;
      if (status !== project.status) patch.status = status;

      if (Object.keys(patch).length > 0) {
        await updateProject(project.id, patch);
        updateProjectStore(project.id, patch);
      }
      setDirty(false);
      toast.success("Project saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save project");
    } finally {
      setSaving(false);
    }
  }, [name, description, status, project, updateProjectStore]);

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <span
          className="h-4 w-4 rounded-sm shrink-0"
          style={{ backgroundColor: project.color || "#6b7280" }}
        />
        <Input
          value={name}
          onChange={(e) => {
            setName(e.target.value);
            setDirty(true);
          }}
          className="text-lg font-semibold h-9 px-2.5"
        />
        <Select
          value={status}
          onValueChange={(v) => {
            setStatus(v as ProjectStatus);
            setDirty(true);
          }}
        >
          <SelectTrigger className="w-[140px] data-[size=default]:h-9 cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {STATUS_OPTIONS.map((opt) => (
              <SelectItem key={opt.value} value={opt.value} className="cursor-pointer">
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {dirty && (
          <Button size="sm" onClick={handleSave} disabled={saving} className="cursor-pointer">
            <IconDeviceFloppy className="h-4 w-4 mr-1" />
            {saving ? "Saving..." : "Save"}
          </Button>
        )}
      </div>
      <Textarea
        value={description}
        onChange={(e) => {
          setDescription(e.target.value);
          setDirty(true);
        }}
        placeholder="Add a description..."
        className="min-h-[60px] text-sm resize-none"
      />
    </div>
  );
}
