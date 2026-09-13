"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { SETTINGS_TYPOGRAPHY } from "@/components/settings/settings-typography";
import { settingsWithDockerAcknowledgement } from "@/hooks/domains/system/use-storage-maintenance";
import { DedicatedDockerDialog, ExternalGoCacheDialog } from "./storage-confirmation-dialogs";
import {
  DockerSection,
  GoCacheSection,
  QuarantineSection,
  ScheduleSection,
  TemporaryArtifactsSection,
  WorkspaceSection,
} from "./storage-policy-card";
import type { PolicySectionProps, StoragePolicyCardProps } from "./storage-policy-card";

export function StoragePolicyCard({
  settings,
  savedSettings,
  capabilities,
  pending,
  pendingReason,
  onChange,
  onAdopt,
  onCleanDependencies,
  onCleanTemporaryArtifacts,
}: StoragePolicyCardProps) {
  const { t } = useTranslation();
  const [dockerDialogOpen, setDockerDialogOpen] = useState(false);
  const [adoptionDialogOpen, setAdoptionDialogOpen] = useState(false);
  const savedAdoptionPath = savedSettings.go_cache.adopted_path;
  const [adoptionPath, setAdoptionPath] = useState(savedAdoptionPath);
  const previousSavedAdoptionPath = useRef(savedAdoptionPath);

  useEffect(() => {
    const previousPath = previousSavedAdoptionPath.current;
    setAdoptionPath((currentPath) =>
      currentPath === previousPath ? savedAdoptionPath : currentPath,
    );
    previousSavedAdoptionPath.current = savedAdoptionPath;
  }, [savedAdoptionPath]);

  const sectionProps: PolicySectionProps = {
    settings,
    savedSettings,
    capabilities,
    pending,
    pendingReason,
    onChange,
  };

  return (
    <section className="min-w-0 space-y-4" data-testid="storage-policy-card">
      <div>
        <h2 className={SETTINGS_TYPOGRAPHY.sectionTitle}>{t("system:storagePolicyTitle")}</h2>
        <p className={SETTINGS_TYPOGRAPHY.sectionDescription}>
          {t("system:storagePolicyDescription")}
        </p>
      </div>
      <div className="space-y-3">
        <ScheduleSection {...sectionProps} />
        <WorkspaceSection {...sectionProps} onCleanDependencies={onCleanDependencies} />
        <GoCacheSection
          {...sectionProps}
          adoptionPath={adoptionPath}
          setAdoptionPath={setAdoptionPath}
          onOpenAdoption={() => setAdoptionDialogOpen(true)}
        />
        <DockerSection {...sectionProps} onOpenDedicated={() => setDockerDialogOpen(true)} />
        <QuarantineSection {...sectionProps} />
        <TemporaryArtifactsSection
          {...sectionProps}
          onCleanTemporaryArtifacts={onCleanTemporaryArtifacts}
        />
      </div>
      <DedicatedDockerDialog
        open={dockerDialogOpen}
        onOpenChange={setDockerDialogOpen}
        onConfirm={() => {
          const next = settingsWithDockerAcknowledgement(settings, true);
          onChange(next);
          setDockerDialogOpen(false);
        }}
      />
      <ExternalGoCacheDialog
        path={adoptionPath}
        open={adoptionDialogOpen}
        onOpenChange={setAdoptionDialogOpen}
        onConfirm={() => {
          void onAdopt(adoptionPath.trim());
          setAdoptionDialogOpen(false);
        }}
      />
    </section>
  );
}
