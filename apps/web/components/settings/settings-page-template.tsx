"use client";

import { CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { SettingsCard } from "@/components/settings/settings-card";
import {
  SettingsSaveDirtyScope,
  useSettingsSaveContributor,
  type SettingsSaveRevision,
} from "@/components/settings/settings-save-provider";

type SettingsPageTemplateProps = {
  title: string;
  description?: string;
  /**
   * Replaces the default title/description block. For a page that is a *section*
   * of a larger one rather than a page in its own right — the workspace Secrets
   * tab, which heads itself like its five sibling tabs instead of like a
   * top-level settings page. `title` is still required: it names the save
   * contributor.
   */
  header?: React.ReactNode;
  isDirty: boolean;
  cardIsDirty?: boolean;
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
  header,
  isDirty,
  cardIsDirty = isDirty,
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
      {header ?? (
        <div>
          <h2 className="text-2xl font-bold">{title}</h2>
          {description && <p className="text-sm text-muted-foreground mt-1">{description}</p>}
        </div>
      )}

      <Separator />

      <SettingsSaveDirtyScope>
        {(nestedIsDirty) => (
          <SettingsCard isDirty={cardIsDirty || nestedIsDirty}>
            <CardContent className="">{children}</CardContent>
          </SettingsCard>
        )}
      </SettingsSaveDirtyScope>

      {deleteSection}
    </div>
  );
}
