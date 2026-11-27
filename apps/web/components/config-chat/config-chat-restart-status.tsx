import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useConfigChatRestart } from "./use-config-chat-restart";

export function ConfigChatRestartStatus({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const restart = useConfigChatRestart(workspaceId);
  return (
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 p-6 text-center text-sm text-muted-foreground">
      <p role={restart.isRestarting ? "status" : "alert"}>
        {restart.isRestarting ? t("configChat:restartingSession") : restart.restartError}
      </p>
      {!restart.isRestarting && (
        <Button
          variant="outline"
          className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onClick={() => void restart.refreshRestart()}
        >
          {t("configChat:refreshSessionStatus")}
        </Button>
      )}
    </div>
  );
}
