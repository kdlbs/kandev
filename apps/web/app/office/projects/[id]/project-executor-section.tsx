"use client";

import { useState, useCallback } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconDeviceFloppy } from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
import { updateProject } from "@/lib/api/domains/office-api";
import { useAppStore } from "@/components/state-provider";
import type { Project } from "@/lib/state/slices/office/types";
import { useTranslation } from "react-i18next";

type ProjectExecutorSectionProps = {
  project: Project;
};

function ExecutorTypeSelect({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      <Label className="text-xs">{t("office:type")}</Label>
      <Select value={value || "inherit"} onValueChange={(v) => onChange(v === "inherit" ? "" : v)}>
        <SelectTrigger className="cursor-pointer">
          <SelectValue placeholder={t("office:inheritFromWorkspace")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="inherit" className="cursor-pointer">
            {t("office:inheritFromWorkspace")}
          </SelectItem>
          <SelectItem value="local_pc" className="cursor-pointer">
            {t("office:localStandalone")}
          </SelectItem>
          <SelectItem value="local_docker" className="cursor-pointer">
            {t("office:localDocker")}
          </SelectItem>
          <SelectItem value="sprites" className="cursor-pointer">
            {t("office:spritesRemoteSandbox")}
          </SelectItem>
          <SelectItem value="remote_docker" className="cursor-pointer">
            {t("office:remoteDocker")}
          </SelectItem>
        </SelectContent>
      </Select>
    </div>
  );
}

function ContainerFields({
  image,
  memoryMb,
  cpuCores,
  onImageChange,
  onMemoryChange,
  onCpuChange,
}: {
  image: string;
  memoryMb: string;
  cpuCores: string;
  onImageChange: (v: string) => void;
  onMemoryChange: (v: string) => void;
  onCpuChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="space-y-1">
        <Label className="text-xs">{t("office:dockerImage")}</Label>
        <Input
          placeholder={t("office:dockerImageExample")}
          value={image}
          onChange={(e) => onImageChange(e.target.value)}
        />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1">
          <Label className="text-xs">{t("office:memoryMb")}</Label>
          <Input
            type="number"
            placeholder="4096"
            value={memoryMb}
            onChange={(e) => onMemoryChange(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label className="text-xs">{t("office:cpuCores")}</Label>
          <Input
            type="number"
            placeholder="2"
            value={cpuCores}
            onChange={(e) => onCpuChange(e.target.value)}
          />
        </div>
      </div>
    </>
  );
}

function buildConfig(input: {
  executorType: string;
  image: string;
  memoryMb: string;
  cpuCores: string;
  isContainer: boolean;
}): Record<string, unknown> {
  const config: Record<string, unknown> = {};
  if (input.executorType) config.type = input.executorType;
  if (input.image) config.image = input.image;
  if (input.isContainer && (input.memoryMb || input.cpuCores)) {
    const limits: Record<string, number> = {};
    if (input.memoryMb) limits.memory_mb = parseInt(input.memoryMb, 10);
    if (input.cpuCores) limits.cpu_cores = parseInt(input.cpuCores, 10);
    config.resource_limits = limits;
  }
  return config;
}

export function ProjectExecutorSection({ project }: ProjectExecutorSectionProps) {
  const { t } = useTranslation();
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
      const newConfig = buildConfig({ executorType, image, memoryMb, cpuCores, isContainer });
      await updateProject(project.id, { executorConfig: newConfig });
      updateProjectStore(project.workspaceId, project.id, { executorConfig: newConfig });
      setDirty(false);
      toast.success(t("office:executorConfigurationSaved"));
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t("office:failedToSaveExecutorConfiguration"),
      );
    } finally {
      setSaving(false);
    }
  }, [
    executorType,
    image,
    memoryMb,
    cpuCores,
    isContainer,
    project.id,
    updateProjectStore,
    project.workspaceId,
  ]);

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold">{t("office:executorConfiguration")}</h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            {t("office:howAgentSessionsRunForThis")}
          </p>
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
            {t("common:save")}
          </Button>
        )}
      </div>

      <div className="space-y-3">
        <ExecutorTypeSelect
          value={executorType}
          onChange={(v) => {
            setExecutorType(v);
            setDirty(true);
          }}
        />
        {isContainer && (
          <ContainerFields
            image={image}
            memoryMb={memoryMb}
            cpuCores={cpuCores}
            onImageChange={(v) => {
              setImage(v);
              setDirty(true);
            }}
            onMemoryChange={(v) => {
              setMemoryMb(v);
              setDirty(true);
            }}
            onCpuChange={(v) => {
              setCpuCores(v);
              setDirty(true);
            }}
          />
        )}
      </div>
    </div>
  );
}
