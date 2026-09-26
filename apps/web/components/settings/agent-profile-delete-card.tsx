"use client";

import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import type { AgentProfile } from "@/lib/types/http";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useConfirmationBoundary } from "@/components/confirmation/mobile-action-confirmation";
import { AgentProfileDeleteConfirmation } from "@/components/settings/agent-profile-delete-dialog";

type DeleteProfileCardProps = {
  profile: AgentProfile;
  onDelete: () => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<void>;
};

export function DeleteProfileCard({
  profile,
  onDelete,
  open,
  onOpenChange,
  onConfirm,
}: DeleteProfileCardProps) {
  const { t } = useTranslation();
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  useConfirmationBoundary(open, profile.id, onOpenChange);
  const deleteAnchorRef = useRef<HTMLButtonElement>(null);
  const closeDeleteConfirmation = () => {
    onOpenChange(false);
    queueMicrotask(() => deleteAnchorRef.current?.focus());
  };
  return (
    <Card className="border-destructive">
      <CardHeader>
        <CardTitle className="text-destructive">{t("agents:deleteProfile")}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium">{t("agents:removeThisProfile")}</p>
          <p className="text-xs text-muted-foreground">{t("agents:actionCannotBeUndone")}</p>
        </div>
        {!open || isFinePointer || isMobile ? (
          <Button
            ref={deleteAnchorRef}
            variant="destructive"
            className="cursor-pointer"
            onClick={onDelete}
            data-testid="profile-delete-trigger"
          >
            <IconTrash className="h-4 w-4 mr-2" />
            {t("agents:delete")}
          </Button>
        ) : null}
        {!isMobile && !isFinePointer && open ? (
          <div className="basis-full min-w-0">
            <AgentProfileDeleteConfirmation
              profileId={profile.id}
              profileName={profile.name}
              open={open}
              isFinePointer={false}
              anchorRef={deleteAnchorRef}
              onOpenChange={onOpenChange}
              onCancel={closeDeleteConfirmation}
              onConfirm={onConfirm}
            />
          </div>
        ) : null}
      </CardContent>
      {isMobile || isFinePointer ? (
        <AgentProfileDeleteConfirmation
          profileId={profile.id}
          profileName={profile.name}
          open={open}
          isFinePointer
          anchorRef={deleteAnchorRef}
          onOpenChange={onOpenChange}
          onCancel={closeDeleteConfirmation}
          onConfirm={onConfirm}
        />
      ) : null}
    </Card>
  );
}
