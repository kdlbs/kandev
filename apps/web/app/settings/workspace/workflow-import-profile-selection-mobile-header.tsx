"use client";

import { useTranslation } from "react-i18next";
import { IconChevronLeft } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DrawerDescription, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import type { WorkflowImportProfileStep } from "@/lib/types/http";

/** Catalog keys for the import review header: profile resolution, or notices only. */
export function importSelectionCopyKeys(reviewOnly: boolean) {
  return reviewOnly
    ? { title: "workflows:importReviewTitle", description: "workflows:importReviewDescription" }
    : {
        title: "workflows:importResolveProfilesTitle",
        description: "workflows:importResolveProfilesDescription",
      };
}

export function MobileSelectionHeader({
  mobileView,
  activeStep,
  reviewOnly = false,
  onBack,
}: {
  mobileView: "list" | "picker";
  activeStep?: WorkflowImportProfileStep;
  /** No profile steps to resolve: the drawer only shows import notices. */
  reviewOnly?: boolean;
  onBack: () => void;
}) {
  const { t } = useTranslation();
  const copy = importSelectionCopyKeys(reviewOnly);
  return (
    <DrawerHeader className="shrink-0 border-b border-border/70 pb-3 text-left">
      <div className="flex items-center gap-2">
        {mobileView === "picker" && (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            className="h-11 w-11 shrink-0 cursor-pointer"
            aria-label={t("common:back")}
            onClick={onBack}
          >
            <IconChevronLeft className="h-4 w-4" aria-hidden="true" />
          </Button>
        )}
        <div className="min-w-0">
          <DrawerTitle>
            {mobileView === "picker" ? t("workflows:importProfilePickerTitle") : t(copy.title)}
          </DrawerTitle>
          <DrawerDescription>
            {mobileView === "picker" && activeStep
              ? t("workflows:importProfilePickerDescription", {
                  stepName: activeStep.step_name,
                })
              : t(copy.description)}
          </DrawerDescription>
        </div>
      </div>
    </DrawerHeader>
  );
}
