"use client";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useRouter } from "@/lib/routing/client-router";
import type { SSHRoute } from "@/lib/types/http-ssh-connections";
import { useSSHSetupContinuation } from "./setup-continuation";
import { settingsActionClassName } from "../settings-control";

export function SetupReturnActions({ route, busy }: { route?: SSHRoute; busy: boolean }) {
  const { t } = useTranslation();
  const flow = useSSHSetupContinuation();
  const router = useRouter();
  if (!flow.draft) return null;
  const editor = flow.draft.editor;
  const ready = route && route.destination.verified_route === route.stamp;
  return (
    <div className="flex flex-wrap gap-2">
      <Button
        type="button"
        variant="outline"
        className={settingsActionClassName()}
        disabled={busy}
        onClick={() => router.push(editor)}
      >
        {t("settings:sshManager.binding.returnToExecutor")}
      </Button>
      {ready && (
        <Button
          type="button"
          className={settingsActionClassName()}
          disabled={busy}
          onClick={() =>
            router.push(editor, { onNavigated: () => flow.select(route.destination.id) })
          }
        >
          {t("settings:sshManager.binding.useConnection")}
        </Button>
      )}
    </div>
  );
}
