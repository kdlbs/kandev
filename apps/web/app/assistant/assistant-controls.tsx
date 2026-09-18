import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import type { AssistantBinding, AssistantMode } from "@/lib/api/domains/assistant-api";
import {
  useAssistantActions,
  assistantFailureKey,
} from "@/hooks/domains/orchestration/use-assistant-actions";
export function AssistantControls({
  binding,
  refresh,
}: {
  binding: AssistantBinding;
  refresh: () => void;
}) {
  const { t } = useTranslation();
  const actions = useAssistantActions(binding, refresh);
  const [confirm, setConfirm] = useState(false);
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-end gap-2">
        <div className="min-w-36 flex-1 md:flex-none">
          <Label htmlFor="assistant-mode">{t("orchestration:assistantMode")}</Label>
          <Select
            value={binding.execution_mode}
            disabled={actions.busy}
            onValueChange={(value) => void actions.mode(value as AssistantMode)}
          >
            <SelectTrigger id="assistant-mode" className="cursor-pointer max-md:min-h-11">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(["answer", "inspect", "design", "execute"] as const).map((mode) => (
                <SelectItem key={mode} value={mode}>
                  {t(`orchestration:assistantMode_${mode}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button
          variant="outline"
          className="cursor-pointer max-md:min-h-11"
          disabled={actions.busy}
          onClick={() => void actions.control(binding.paused ? "resume" : "pause")}
        >
          {t(binding.paused ? "orchestration:assistantResume" : "orchestration:assistantPause")}
        </Button>
        <Button
          variant="outline"
          className="cursor-pointer max-md:min-h-11"
          disabled={actions.busy}
          onClick={() => setConfirm(true)}
        >
          {t("orchestration:assistantStop")}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        {t(`orchestration:assistantModeHint_${binding.execution_mode}`)}
      </p>
      {binding.paused && (
        <p role="status" className="text-sm">
          {t("orchestration:assistantPaused")}
        </p>
      )}
      {Boolean(actions.error) && (
        <p role="alert" className="text-sm">
          {t(assistantFailureKey(actions.error))}
        </p>
      )}
      <ControlReceipt actions={actions} />
      <AlertDialog open={confirm} onOpenChange={setConfirm}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("orchestration:assistantStop")}</AlertDialogTitle>
            <AlertDialogDescription>{t("orchestration:assistantStopHint")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="cursor-pointer max-md:min-h-11">
              {t("common:cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              className="cursor-pointer max-md:min-h-11"
              onClick={() => void actions.control("stop_managed_work")}
            >
              {t("orchestration:assistantStop")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function ControlReceipt({ actions }: { actions: ReturnType<typeof useAssistantActions> }) {
  const { t } = useTranslation();
  return (
    <>
      {actions.receipt?.sessions && (
        <div className="space-y-2 text-sm" role="status">
          <p>
            {t(
              actions.receipt.partial
                ? "orchestration:assistantStopPartial"
                : "orchestration:assistantStopReported",
            )}
          </p>
          <ul className="space-y-1">
            {actions.receipt.sessions.map((row, index) => (
              <li key={`${row.task_id}:${row.session_id}:${index}`} className="break-all">
                {row.session_id || row.task_id}: {t(`orchestration:assistantStop_${row.status}`)}
              </li>
            ))}
          </ul>
          {actions.receipt.next_cursor && (
            <Button
              variant="outline"
              disabled={actions.busy}
              className="cursor-pointer max-md:min-h-11"
              onClick={() =>
                void actions.control("stop_managed_work", actions.receipt?.next_cursor)
              }
            >
              {t("orchestration:assistantStopContinue")}
            </Button>
          )}
        </div>
      )}
    </>
  );
}
