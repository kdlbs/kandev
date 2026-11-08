import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useTranslation } from "react-i18next";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { SecretListItem } from "@/lib/types/http-secrets";

const EMPTY_SECRET = "__mcp_no_secret__";

export function MCPSecretSelect({
  id,
  value,
  items,
  loading,
  onChange,
}: {
  id: string;
  value: string;
  items: SecretListItem[];
  loading: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Select
      value={value || EMPTY_SECRET}
      onValueChange={(next) => onChange(next === EMPTY_SECRET ? "" : next)}
    >
      <SelectTrigger
        id={id}
        disabled={loading}
        className={controlSizingClassName("standard", "min-w-0 w-full cursor-pointer")}
      >
        <SelectValue placeholder={t("executors:selectASecret")} />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={EMPTY_SECRET}>{t("executors:none")}</SelectItem>
        {value && !items.some((item) => item.id === value) && (
          <SelectItem value={value}>{t("executors:missingSecretReference")}</SelectItem>
        )}
        {items.map((item) => (
          <SelectItem key={item.id} value={item.id}>
            {item.name} (
            {item.scope === "workspace"
              ? t("workspaces:workspaceSecretScope")
              : t("workspaces:globalSecretScope")}
            )
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
