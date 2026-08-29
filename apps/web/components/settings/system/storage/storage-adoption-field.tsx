import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { useTranslation } from "react-i18next";
import { StorageActionButton } from "./storage-action-button";
import { StorageSettingHelp } from "./storage-setting-help";

type Props = {
  path: string;
  setPath: (path: string) => void;
  onOpen: () => void;
  pending: boolean;
  /** Overrides the default pending explanation, e.g. for the admin gate. */
  pendingReason?: string;
  enabled: boolean;
};

export function StorageAdoptionField({
  path,
  setPath,
  onOpen,
  pending,
  pendingReason,
  enabled,
}: Props) {
  const { t } = useTranslation();
  let disabledReason: string | undefined;
  if (pending) disabledReason = pendingReason ?? t("system:storageActionPending");
  else if (!enabled) disabledReason = t("system:storageEnableGoCacheFirst");
  else if (!path.trim()) disabledReason = t("system:storageEnterCachePathFirst");
  const fieldLabel = t("system:storageExternalGoCache");
  return (
    <div className="min-w-0 space-y-2 pt-3">
      <div className="flex items-center gap-1">
        <Label htmlFor="storage-adoption-path">{fieldLabel}</Label>
        <StorageSettingHelp label={fieldLabel}>
          {t("system:storageExternalGoCacheHelp")}
        </StorageSettingHelp>
      </div>
      <p className="text-xs text-muted-foreground">
        {t("system:storageExternalGoCacheDescription")}
      </p>
      <div className="flex min-w-0 flex-col gap-2 sm:flex-row">
        <Input
          id="storage-adoption-path"
          value={path}
          disabled={pending || !enabled}
          onChange={(event) => setPath(event.target.value)}
          placeholder="/root/.cache/go-build"
          className="h-11 min-w-0 font-mono"
          data-testid="storage-go-cache-adopt-path"
        />
        <StorageActionButton
          variant="outline"
          disabledReason={disabledReason}
          onClick={onOpen}
          data-testid="storage-go-cache-adopt"
        >
          {t("system:storageAdoptCache")}
        </StorageActionButton>
      </div>
    </div>
  );
}
