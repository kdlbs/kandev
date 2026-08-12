"use client";

import { IconAlertTriangle, IconCloudUpload } from "@tabler/icons-react";
import {
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from "@kandev/ui/dropdown-menu";
import { useTranslation } from "react-i18next";

export function PushSubmenu({
  disabled,
  disabledReason,
  onPush,
}: {
  disabled: boolean;
  disabledReason?: string;
  onPush: (force: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger
        className="cursor-pointer gap-3"
        disabled={disabled}
        title={disabledReason}
      >
        <IconCloudUpload className="h-4 w-4 text-green-500" />
        <span className="flex-1">{t("task:push")}</span>
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent>
        <DropdownMenuItem
          className="cursor-pointer gap-3"
          onClick={() => onPush(false)}
          disabled={disabled}
          title={disabledReason}
        >
          <IconCloudUpload className="h-4 w-4 text-green-500" />
          <span>{t("task:push")}</span>
        </DropdownMenuItem>
        <DropdownMenuItem
          className="cursor-pointer gap-3"
          onClick={() => onPush(true)}
          disabled={disabled}
          title={disabledReason}
        >
          <IconAlertTriangle className="h-4 w-4 text-orange-500" />
          <span>{t("task:forcePush")}</span>
        </DropdownMenuItem>
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}
