import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { mcpSelectionErrorKey } from "./mcp-selection-feedback";

export function MCPSelectionSaveFeedback({
  error,
  reload,
}: {
  error: unknown;
  reload: () => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const key = mcpSelectionErrorKey(error);
  if (!key) return null;
  return (
    <div className="mt-2 space-y-2">
      <p className="text-sm text-destructive" role="alert">
        {t(key)}
      </p>
      <Button type="button" variant="outline" onClick={() => void reload()}>
        {t("settings:mcpReloadChoices")}
      </Button>
    </div>
  );
}
