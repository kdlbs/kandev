import { BulkSessionRemoveDialog } from "./session-tab-menu";
import type { BulkSessionRemovalScope } from "./session-bulk-removal";

export function SessionTabBulkRemovalDialog({
  scope,
  count,
  pending,
  removedCount,
  onCancel,
  onConfirm,
}: {
  scope: BulkSessionRemovalScope | null;
  count: number;
  pending: boolean;
  removedCount: number;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <BulkSessionRemoveDialog
      scope={scope}
      count={count}
      pending={pending}
      removedCount={removedCount}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  );
}
