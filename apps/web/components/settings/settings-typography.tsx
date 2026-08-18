import type { ComponentProps, ReactNode } from "react";
import { Label } from "@kandev/ui/label";

import { cn } from "@/lib/utils";

/**
 * The settings role scale. Keep domain-specific code, status, and badge styles
 * outside this map so compact technical UI remains intentional.
 */
export const SETTINGS_TYPOGRAPHY = {
  pageTitle: "text-2xl font-bold leading-tight",
  pageDescription: "mt-1 text-sm text-muted-foreground",
  sectionTitle: "flex items-center gap-2 text-lg font-semibold leading-tight",
  sectionDescription: "mt-1 text-sm text-muted-foreground",
  cardTitle: "text-base font-semibold leading-5",
  cardDescription: "text-sm/relaxed text-muted-foreground",
  fieldLabel: "text-xs/relaxed font-medium text-foreground",
  fieldDescription: "text-xs/relaxed text-muted-foreground",
  error: "text-sm/relaxed text-destructive",
  meta: "text-[10px]/relaxed text-muted-foreground",
  control: "text-sm md:text-xs",
  mobileAction: "min-h-11 text-sm md:min-h-7 md:text-xs",
} as const;

type SettingsFieldDescriptionProps = ComponentProps<"p">;

export function SettingsFieldLabel({ className, ...props }: ComponentProps<typeof Label>) {
  return <Label className={cn(SETTINGS_TYPOGRAPHY.fieldLabel, className)} {...props} />;
}

export function SettingsFieldDescription({ className, ...props }: SettingsFieldDescriptionProps) {
  return <p className={cn(SETTINGS_TYPOGRAPHY.fieldDescription, className)} {...props} />;
}

export function SettingsErrorText({ className, ...props }: ComponentProps<"p">) {
  return <p role="alert" className={cn(SETTINGS_TYPOGRAPHY.error, className)} {...props} />;
}

export type SettingsFieldProps = {
  label: ReactNode;
  helper?: ReactNode;
  error?: ReactNode;
  labelProps?: ComponentProps<typeof Label>;
  className?: string;
  children: ReactNode;
};

/** Shared field composition keeps label, helper, and validation roles together. */
export function SettingsField({
  label,
  helper,
  error,
  labelProps,
  className,
  children,
}: SettingsFieldProps) {
  return (
    <div className={cn("space-y-2", className)}>
      <SettingsFieldLabel {...labelProps}>{label}</SettingsFieldLabel>
      {children}
      {helper && <SettingsFieldDescription>{helper}</SettingsFieldDescription>}
      {error && <SettingsErrorText>{error}</SettingsErrorText>}
    </div>
  );
}

export type SettingsPageHeaderProps = {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  titleTestId?: string;
  className?: string;
};

export function SettingsPageHeader({
  title,
  description,
  actions,
  titleTestId,
  className,
}: SettingsPageHeaderProps) {
  return (
    <div
      className={cn("flex flex-col gap-3 md:flex-row md:items-start md:justify-between", className)}
    >
      <div className="min-w-0">
        <h2 className={SETTINGS_TYPOGRAPHY.pageTitle} data-testid={titleTestId}>
          {title}
        </h2>
        {description && <p className={SETTINGS_TYPOGRAPHY.pageDescription}>{description}</p>}
      </div>
      {actions && <div className="w-full shrink-0 md:w-auto">{actions}</div>}
    </div>
  );
}
