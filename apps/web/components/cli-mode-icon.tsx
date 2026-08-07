import { IconTerminal2 } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";

// Marks an agent profile that runs in CLI passthrough mode — the prompt is
// auto-injected into a raw terminal rather than sent as a structured request.
// Shown next to passthrough profiles in the manual create dialog and the
// Linear/Jira/GitHub watcher dialogs so the two paths stay visually consistent.
export function CliModeIcon({ className }: { className?: string }) {
  const { t } = useTranslation();
  return (
    <IconTerminal2
      className={className ?? "size-3.5 text-muted-foreground"}
      title={t("common:cliModeYourPromptWillBe")}
    />
  );
}
