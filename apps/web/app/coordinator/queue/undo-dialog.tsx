import { useTranslation } from "react-i18next";
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
import type { ActivityItem } from "@/lib/api/domains/coordinator-activity-api";
import { undoDialogSentence } from "./activity-text";

export type UndoDialogProps = {
  /** The row being undone, as it was when the dialog opened. Null while closed. */
  item: ActivityItem | null;
  stepName: string | undefined;
  onConfirm: (item: ActivityItem) => void;
  onClose: () => void;
  onCloseAutoFocus: (event: Event) => void;
};

/**
 * The "Undo this?" confirmation. Cancel has initial focus and Enter activates
 * whichever button is focused (no default confirm); Escape and Cancel send
 * nothing, and a click outside does nothing.
 */
export function UndoDialog({
  item,
  stepName,
  onConfirm,
  onClose,
  onCloseAutoFocus,
}: UndoDialogProps) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={item !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent enterConfirms={false} onCloseAutoFocus={onCloseAutoFocus}>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("coordinator:activityUndoTitle")}</AlertDialogTitle>
          <AlertDialogDescription data-testid="activity-undo-dialog-text">
            {item ? undoDialogSentence(item, stepName, t) : null}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">
            {t("coordinator:activityUndoCancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            className="cursor-pointer"
            onClick={() => item && onConfirm(item)}
            data-testid="activity-undo-confirm"
          >
            {t("coordinator:activityUndoConfirm")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
