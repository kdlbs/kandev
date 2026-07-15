"use client";

import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import {
  useSettingsSaveContributor,
  type SettingsSaveRevision,
} from "@/components/settings/settings-save-provider";

type SettingsPageTemplateProps = {
  title: string;
  description?: string;
  isDirty: boolean;
  saveStatus: "idle" | "loading" | "success" | "error";
  onSave: () => Promise<unknown> | void;
  saveId?: string;
  saveRevision?: SettingsSaveRevision;
  canSave?: boolean;
  invalidReason?: string;
  onDiscard?: () => void;
  showSaveButton?: boolean;
  children: React.ReactNode;
  deleteSection?: React.ReactNode;
};

export function SettingsPageTemplate({
  title,
  description,
  isDirty,
  onSave,
  saveId,
  saveRevision = 0,
  canSave = true,
  invalidReason,
  onDiscard,
  showSaveButton = true,
  children,
  deleteSection,
}: SettingsPageTemplateProps) {
  useSettingsSaveContributor({
    id: saveId ?? `settings-page:${title}`,
    revision: saveRevision,
    isDirty: showSaveButton && isDirty,
    canSave,
    invalidReason,
    save: async () => {
      await onSave();
    },
    discard: onDiscard ?? (() => undefined),
  });

  return (
    <div className="space-y-8">
      <div>
        <div>
          <h2 className="text-2xl font-bold">{title}</h2>
          {description && <p className="text-sm text-muted-foreground mt-1">{description}</p>}
        </div>
      </div>

      <Separator />

      <Card className={isDirty ? "border border-yellow-500/60" : "border"}>
        <CardContent className="">{children}</CardContent>
      </Card>

      {deleteSection}
    </div>
  );
}
