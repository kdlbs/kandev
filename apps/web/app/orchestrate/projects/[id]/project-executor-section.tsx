"use client";

import { useState, useCallback } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import { IconDeviceFloppy } from "@tabler/icons-react";
import { toast } from "sonner";
import { updateProject } from "@/lib/api/domains/orchestrate-api";
import { useAppStore } from "@/components/state-provider";
import type { Project } from "@/lib/state/slices/orchestrate/types";

type ProjectExecutorSectionProps = {
  project: Project;
};

export function ProjectExecutorSection({ project }: ProjectExecutorSectionProps) {
  const updateProjectStore = useAppStore((s) => s.updateProject);
  const config = project.executorConfig ?? {};

  const [executorType, setExecutorType] = useState((config.type as string) ?? "");
  const [image, setImage] = useState((config.image as string) ?? "");
  const [memoryMb, setMemoryMb] = useState(
    String((config.resource_limits as Record<string, number>)?.memory_mb ?? ""),
  );
  const [cpuCores, setCpuCores] = useState(
    String((config.resource_limits as Record<string, number>)?.cpu_cores ?? ""),
  );
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);

  const isContainer = executorType === "local_docker" || executorType === "remote_docker";

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const newConfig: Record<string, unknown> = {};
      if (executorType) newConfig.type = executorType;
      if (image) newConfig.image = image;
      if (isContainer && (memoryMb || cpuCores)) {
        const limits: Record<string, number> = {};
        if (memoryMb) limits.memory_mb = parseInt(memoryMb, 10);
        if (cpuCores) limits.cpu_cores = parseInt(cpuCores, 10);
        newConfig.resource_limits = limits;
      }
      await updateProject(project.id, { executorConfig: newConfig });
      updateProjectStore(project.id, { executorConfig: newConfig });
      setDirty(false);
      toast.success("Executor configuration saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save executor configuration");
    } finally {
      setSaving(false);
    }
  }, [executorType, image, memoryMb, cpuCores, isContainer, project.id, updateProjectStore]);

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold">Executor Configuration</h2>
          <p className="text-xs text-muted-foreground mt-0.5">How agent sessions run for this project.</p>
        </div>
        {dirty && (
          <Button
            size="sm"
            variant="outline"
            onClick={handleSave}
            disabled={saving}
            className="cursor-pointer"
          >
            <IconDeviceFloppy className="h-3.5 w-3.5 mr-1" />
            Save
          </Button>
        )}
      </div>

      <div className="space-y-3">
        <div className="space-y-1">
          <Label className="text-xs">Type</Label>
          <Select
            value={executorType || "inherit"}
            onValueChange={(v) => {
              setExecutorType(v === "inherit" ? "" : v);
              setDirty(true);
            }}
          >
            <SelectTrigger className="cursor-pointer">
              <SelectValue placeholder="Inherit from workspace" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="inherit" className="cursor-pointer">Inherit from workspace</SelectItem>
              <SelectItem value="local_pc" className="cursor-pointer">Local (standalone)</SelectItem>
              <SelectItem value="local_docker" className="cursor-pointer">Local Docker</SelectItem>
              <SelectItem value="sprites" className="cursor-pointer">Sprites (remote sandbox)</SelectItem>
              <SelectItem value="remote_docker" className="cursor-pointer">Remote Docker</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {isContainer && (
          <>
            <div className="space-y-1">
              <Label className="text-xs">Docker Image</Label>
              <Input
                placeholder="e.g. node:20-slim"
                value={image}
                onChange={(e) => { setImage(e.target.value); setDirty(true); }}
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1">
                <Label className="text-xs">Memory (MB)</Label>
                <Input
                  type="number"
                  placeholder="4096"
                  value={memoryMb}
                  onChange={(e) => { setMemoryMb(e.target.value); setDirty(true); }}
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">CPU Cores</Label>
                <Input
                  type="number"
                  placeholder="2"
                  value={cpuCores}
                  onChange={(e) => { setCpuCores(e.target.value); setDirty(true); }}
                />
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
