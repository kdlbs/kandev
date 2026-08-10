"use client";

import { Button } from "@kandev/ui/button";
import type { OnboardingFSWorkspace } from "@/lib/api/domains/office-api";
import { CloseButton } from "./close-button";
import { useTranslation } from "react-i18next";

type StepImportProps = {
  fsWorkspaces: OnboardingFSWorkspace[];
  submitting: boolean;
  onImport: () => void;
  onSkip: () => void;
  closeHref: string;
};

export function StepImport({
  fsWorkspaces,
  submitting,
  onImport,
  onSkip,
  closeHref,
}: StepImportProps) {
  const { t } = useTranslation();
  return (
    <div className="fixed inset-0 z-50 bg-background flex items-center justify-center">
      <div className="relative w-full max-w-2xl mx-auto px-6 text-center">
        <CloseButton href={closeHref} />
        <h1 className="text-2xl font-semibold tracking-tight">
          {t("office:existingConfigurationFound")}
        </h1>
        {/*
          One key with `count`, not "workspace" + a hand-written "s". The plural
          rule belongs in the catalog: English picks between two forms, other
          languages between up to six.
        */}
        <p className="mt-2 text-muted-foreground">
          {t("office:foundWorkspacesOnFilesystem", { count: fsWorkspaces.length })}
        </p>
        <div className="mt-6 rounded-lg border bg-muted/50 p-4">
          <ul className="space-y-1 text-sm text-left">
            {fsWorkspaces.map((ws) => (
              <li key={ws.name} className="flex items-center gap-2">
                <span className="h-1.5 w-1.5 rounded-full bg-primary" />
                {ws.name}
              </li>
            ))}
          </ul>
        </div>
        <div className="mt-8 flex items-center justify-center gap-3">
          <Button
            variant="outline"
            onClick={onSkip}
            disabled={submitting}
            className="cursor-pointer"
          >
            {t("office:startFresh")}
          </Button>
          <Button onClick={onImport} disabled={submitting} className="cursor-pointer">
            {submitting ? t("office:importing") : t("office:importContinue")}
          </Button>
        </div>
      </div>
    </div>
  );
}
