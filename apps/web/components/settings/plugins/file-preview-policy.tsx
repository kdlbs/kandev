import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useFilePreviewPolicy } from "@/hooks/use-file-preview-policy";
import { Button } from "@kandev/ui/button";
export function FilePreviewPolicySetting({ pluginId }: { pluginId: string }) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { policy, failed, saving, update } = useFilePreviewPolicy(pluginId, workspaceId);
  return (
    <section className="space-y-2 border-t pt-4">
      <h3 className="text-sm font-medium">{t("plugins:viewerPolicyTitle")}</h3>
      <p className="text-sm text-muted-foreground">{t("plugins:viewerPolicyScope")}</p>
      <div
        role="group"
        aria-label={t("plugins:viewerPolicyTitle")}
        className="flex flex-wrap gap-2"
      >
        {(["ask", "allow", "deny"] as const).map((mode) => (
          <Button
            className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
            key={mode}
            disabled={!policy || saving}
            aria-pressed={policy?.mode === mode}
            variant={policy?.mode === mode ? "default" : "outline"}
            onClick={() => void update(mode)}
          >
            {t(`plugins:viewerPolicy_${mode}`)}
          </Button>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">{t("plugins:viewerPolicyTrust")}</p>
      {failed && <p role="alert">{t("plugins:viewerPolicyFailed")}</p>}
    </section>
  );
}
