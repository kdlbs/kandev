import { useIntegratedControl } from "./integrated-control-provider";
import { ControlAttentionList } from "./control-attention-list";
import { ControlKpis, ControlStatusStrip } from "./control-summary-head";
import { useControlSignals } from "./use-control-signals";

/**
 * The summary head: state sentence, figures, and the tasks that need a person.
 * A selected workspace keeps only its own tasks; the fleet-wide sentence and
 * figures are not shown for it.
 */
export function ControlOverview({ workspaceId }: { workspaceId?: string }) {
  const control = useIntegratedControl();
  const signals = useControlSignals({ workspaceId });
  const names = Object.fromEntries(
    (control?.data?.workspaces ?? []).map((workspace) => [workspace.id, workspace.name]),
  );
  return (
    <div className="space-y-5" data-testid="control-overview">
      {!workspaceId && <ControlStatusStrip signals={signals} />}
      {!workspaceId && <ControlKpis signals={signals} names={names} />}
      <ControlAttentionList items={signals.attention} names={names} />
    </div>
  );
}
