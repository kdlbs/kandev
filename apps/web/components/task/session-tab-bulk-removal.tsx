import { BulkSessionRemoveDialog } from "./session-tab-menu";
import type { BulkSessionRemovalScope } from "./session-bulk-removal";

export function SessionTabBulkRemovalDialog({
  scope,
  count,
  pending,
  onCancel,
  onConfirm,
}: {
  scope: BulkSessionRemovalScope | null;
  count: number;
  pending: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <BulkSessionRemoveDialog
      scope={scope}
      count={count}
      pending={pending}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  );
}
