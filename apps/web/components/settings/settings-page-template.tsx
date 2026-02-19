"use client";

import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { UnsavedChangesBadge, UnsavedSaveButton } from "@/components/settings/unsaved-indicator";

type SettingsPageTemplateProps = {
  title: string;
  description?: string;
  isDirty: boolean;
  saveStatus: "idle" | "loading" | "success" | "error";
  onSave: () => void;
  showSaveButton?: boolean;
  children: React.ReactNode;
  deleteSection?: React.ReactNode;
};

export function SettingsPageTemplate({
  title,
  description,
  isDirty,
  saveStatus,
  onSave,
  showSaveButton = true,
  children,
  deleteSection,
}: SettingsPageTemplateProps) {
  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-2xl font-bold">{title}</h2>
          {description && <p className="text-sm text-muted-foreground mt-1">{description}</p>}
        </div>
        {showSaveButton && (
          <div className="flex items-center gap-3">
            {isDirty && <UnsavedChangesBadge />}
            <UnsavedSaveButton
              isDirty={isDirty}
              isLoading={saveStatus === "loading"}
              status={saveStatus}
              onClick={onSave}
            />
          </div>
        )}
      </div>

      <Separator />

      <Card className={isDirty ? "border border-yellow-500/60" : "border"}>
        <CardContent className="">{children}</CardContent>
      </Card>

      {deleteSection}
    </div>
  );
}
