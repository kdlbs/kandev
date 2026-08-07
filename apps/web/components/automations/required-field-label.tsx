import type { ComponentProps } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { cn } from "@/lib/utils";

export function RequiredFieldLabel({
  children,
  className,
  ...props
}: ComponentProps<typeof Label>) {
  const { t } = useTranslation();
  return (
    <Label className={cn("gap-1.5", className)} {...props}>
      <span>{children}</span>
      <span aria-hidden className="text-destructive">
        *
      </span>
      <span className="sr-only">{t("automations:required")}</span>
    </Label>
  );
}
