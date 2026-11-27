import { Separator } from "@kandev/ui/separator";
import { WorkspaceIdlePolicyCard } from "@/components/settings/workspaces/workspace-idle-policy-card";

type Props = {
  canManage: boolean;
  enabled: boolean;
  timeoutMinutes: string;
  timeoutValid: boolean;
  savedTimeoutMinutes: number;
  enabledIsDirty: boolean;
  timeoutIsDirty: boolean;
  onEnabledChange: (enabled: boolean) => void;
  onTimeoutChange: (timeout: string) => void;
};

export function WorkspaceIdlePolicyFormSection({
  canManage,
  enabled,
  timeoutMinutes,
  timeoutValid,
  savedTimeoutMinutes,
  enabledIsDirty,
  timeoutIsDirty,
  onEnabledChange,
  onTimeoutChange,
}: Props) {
  const handleEnabledChange = (nextEnabled: boolean) => {
    if (!nextEnabled && !timeoutValid) onTimeoutChange(String(savedTimeoutMinutes));
    onEnabledChange(nextEnabled);
  };

  return (
    <>
      <Separator />
      <WorkspaceIdlePolicyCard
        canManage={canManage}
        enabled={enabled}
        timeoutMinutes={timeoutMinutes}
        timeoutValid={timeoutValid}
        enabledIsDirty={enabledIsDirty}
        timeoutIsDirty={timeoutIsDirty}
        onEnabledChange={handleEnabledChange}
        onTimeoutChange={onTimeoutChange}
      />
      <Separator />
    </>
  );
}
