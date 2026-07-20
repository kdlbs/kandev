"use client";

import { IconAlertCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { RequestIndicator } from "@/components/request-indicator";

type UnsavedSaveButtonProps = {
  isDirty: boolean;
  isLoading: boolean;
  status: "idle" | "loading" | "success" | "error";
  onClick: () => void;
  cleanLabel?: string;
  disabled?: boolean;
};

const dirtyButtonClass = "border-success/60 text-success hover:bg-success/10";

export function UnsavedChangesBadge() {
  return <span className="text-xs text-success">Unsaved changes</span>;
}

export function UnsavedSaveButton({
  isDirty,
  isLoading,
  status,
  onClick,
  disabled,
  cleanLabel = "Save",
}: UnsavedSaveButtonProps) {
  return (
    <Button
      type="button"
      size="default"
      variant={isDirty ? "secondary" : "outline"}
      onClick={onClick}
      disabled={isLoading || Boolean(disabled)}
      className={isDirty ? dirtyButtonClass : "cursor-pointer"}
    >
      {isDirty && <IconAlertCircle className="h-4 w-4 mr-2" />}
      {isDirty ? "Save" : cleanLabel}
      {status !== "idle" && (
        <span className="ml-2">
          <RequestIndicator status={status} />
        </span>
      )}
    </Button>
  );
}
