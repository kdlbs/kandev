"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { ApiError } from "@/lib/api/client";
import { addStandingOrder, type StandingOrder } from "@/lib/api/domains/coordinator-api";
import { LONG_TEXT_FIELD_CLASS } from "@/components/coordinators/long-text-field";

const MAX_ORDER_CODE_POINTS = 500;
const LIMIT_ERROR_CODE = "standing_order_limit";

type AddStandingOrderDialogProps = {
  workspaceId: string;
  coordinatorId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialText?: string;
  sourceProposalId?: string;
  onAdded: (order: StandingOrder) => void;
};

function codePointCount(text: string): number {
  return Array.from(text).length;
}

export function AddStandingOrderDialog(props: AddStandingOrderDialogProps) {
  const { workspaceId, coordinatorId, open, onOpenChange, initialText, sourceProposalId, onAdded } =
    props;
  const { t } = useTranslation();
  const [text, setText] = useState(initialText ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const trimmed = text.trim();
  const used = codePointCount(trimmed);
  const canSave = !saving && used > 0 && used <= MAX_ORDER_CODE_POINTS;

  const submit = async () => {
    if (!canSave) return;
    setSaving(true);
    setError(null);
    try {
      const order = await addStandingOrder(workspaceId, coordinatorId, {
        text: trimmed,
        ...(sourceProposalId ? { source_proposal_id: sourceProposalId } : {}),
      });
      onAdded(order);
      onOpenChange(false);
    } catch (err) {
      const isLimit = err instanceof ApiError && err.errorCode === LIMIT_ERROR_CODE;
      setError(
        t(isLimit ? "coordinator:standingOrderLimit" : "coordinator:standingOrderAddFailed"),
      );
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => !saving && onOpenChange(next)}>
      <DialogContent data-testid="add-standing-order-dialog" className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("coordinator:standingOrderDialogTitle")}</DialogTitle>
          <DialogDescription>{t("coordinator:standingOrderDialogDescription")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="standing-order-text">{t("coordinator:standingOrderTextLabel")}</Label>
          <Textarea
            id="standing-order-text"
            className={LONG_TEXT_FIELD_CLASS}
            data-testid="standing-order-text"
            rows={4}
            value={text}
            placeholder={t("coordinator:standingOrderTextPlaceholder")}
            onChange={(event) => setText(event.target.value)}
          />
          <div className="flex items-start justify-between gap-2 text-xs">
            <p role="alert" className="text-destructive">
              {error}
            </p>
            <span className="shrink-0 text-muted-foreground">
              {t("coordinator:standingOrderCounter", { used, max: MAX_ORDER_CODE_POINTS })}
            </span>
          </div>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={saving}
            className="min-h-12 cursor-pointer sm:min-h-9"
            onClick={() => onOpenChange(false)}
          >
            {t("common:cancel")}
          </Button>
          <Button
            type="button"
            disabled={!canSave}
            data-testid="standing-order-save"
            className="min-h-12 cursor-pointer sm:min-h-9"
            onClick={submit}
          >
            {t("coordinator:standingOrderSave")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
