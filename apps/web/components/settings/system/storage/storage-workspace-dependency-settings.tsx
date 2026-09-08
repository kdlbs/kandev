import { Switch } from "@kandev/ui/switch";
import { useTranslation } from "react-i18next";
import type { StorageMaintenanceSettings } from "@/lib/types/system";
import { StorageActionButton } from "./storage-action-button";
import { StorageSettingHelp } from "./storage-setting-help";
import { SettingRow } from "./storage-policy-fields";

// Keep this display snapshot in sync with backend DependencyDirectoryAllowlist.
// i18n-exempt: dependency directory name on disk.
const DEPENDENCY_DIRECTORIES = [
  "node_modules",
  "bower_components",
  ".pnpm-store",
  ".yarn/cache",
  ".yarn/unplugged",
  ".venv",
  "venv",
  ".tox",
  ".nox",
  "__pypackages__",
  "Pods",
  ".gradle",
] as const;

type Props = {
  settings: StorageMaintenanceSettings;
  savedSettings: StorageMaintenanceSettings;
  pending: boolean;
  /** Overrides the default pending explanation, e.g. for the admin gate. */
  pendingReason?: string;
  onChange: (settings: StorageMaintenanceSettings) => void;
  onCleanDependencies?: () => void;
};

export function StorageWorkspaceDependencySettings({
  settings,
  savedSettings,
  pending,
  pendingReason,
  onChange,
  onCleanDependencies,
}: Props) {
  const { t } = useTranslation();
  const dependenciesEnabled = settings.workspaces.dependency_cleanup_enabled;
  const savedDependenciesEnabled = savedSettings.workspaces.dependency_cleanup_enabled;
  const dependencyCleanupDirty = dependenciesEnabled !== savedDependenciesEnabled;
  let dependencyActionDisabledReason: string | undefined;
  if (pending) {
    dependencyActionDisabledReason = pendingReason ?? t("system:storageActionPending");
  } else if (!savedDependenciesEnabled) {
    dependencyActionDisabledReason = t("system:storageEnableWorkspaceDependenciesAndSave");
  }

  return (
    <>
      <SettingRow
        title={t("system:storageWorkspaceDependencies")}
        description={t("system:storageWorkspaceDependenciesDescription")}
        help={t("system:storageWorkspaceDependenciesHelp")}
        control={
          <Switch
            checked={settings.workspaces.dependency_cleanup_enabled}
            disabled={pending || !settings.workspaces.enabled}
            onCheckedChange={(dependency_cleanup_enabled) =>
              onChange({
                ...settings,
                workspaces: { ...settings.workspaces, dependency_cleanup_enabled },
              })
            }
            aria-label={t("system:storageWorkspaceDependencies")}
            data-testid="storage-workspace-dependencies-enabled"
            data-settings-dirty={dependencyCleanupDirty}
          />
        }
      />
      <div className="min-w-0 space-y-2 border-b py-3" data-testid="storage-dependency-allowlist">
        <div className="flex min-w-0 items-center gap-1">
          <p className="text-sm font-medium">{t("system:storageDependencyFoldersTitle")}</p>
          <StorageSettingHelp label={t("system:storageDependencyFoldersTitle")}>
            {t("system:storageDependencyFoldersHelp")}
          </StorageSettingHelp>
        </div>
        <p className="text-xs text-muted-foreground">
          {t("system:storageDependencyFoldersDescription")}
        </p>
        <ul className="grid min-w-0 grid-cols-1 gap-1 text-xs sm:grid-cols-2">
          {DEPENDENCY_DIRECTORIES.map((directory) => (
            <li key={directory} className="min-w-0">
              <code className="block break-all rounded bg-muted px-2 py-1">{directory}</code>
            </li>
          ))}
        </ul>
        {onCleanDependencies && (
          <StorageActionButton
            variant="outline"
            disabledReason={dependencyActionDisabledReason}
            onClick={onCleanDependencies}
            data-testid="storage-clean-workspace-dependencies"
          >
            {t("system:storageCleanWorkspaceDependencies")}
          </StorageActionButton>
        )}
      </div>
    </>
  );
}
