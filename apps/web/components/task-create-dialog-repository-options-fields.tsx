import { useId } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { Textarea } from "@kandev/ui/textarea";
import { Button } from "@kandev/ui/button";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@kandev/ui/select";
import type { RepositoryCheckoutCapabilities } from "@/lib/types/repository-checkout-options";

type Props = {
  onDemand: boolean;
  setOnDemand: (value: boolean) => void;
  selectedFolders: boolean;
  setSelectedFolders: (value: boolean) => void;
  directories: string;
  setDirectories: (value: string) => void;
  invalid: boolean;
  errorLines?: number[];
  capabilities?: RepositoryCheckoutCapabilities;
  failed?: boolean;
  retry: () => void;
};
export function RepositoryOptionsFields(props: Props) {
  const { t } = useTranslation();
  const id = useId();
  const { capabilities } = props;
  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t("task:checkoutOptions.taskOnly")}</p>
      <div className="space-y-2">
        <CheckoutChoice
          id={`${id}-download`}
          label={t("task:checkoutOptions.download")}
          selected={props.onDemand}
          onChange={props.setOnDemand}
          defaultLabel={t("task:checkoutOptions.standard")}
          selectedLabel={t("task:checkoutOptions.onDemand")}
          disabled={!capabilities?.on_demand}
          testId="repository-options-download"
        />
        <p className="text-xs leading-relaxed text-muted-foreground">
          {t("task:checkoutOptions.downloadHelp")}
        </p>
      </div>
      <CheckoutChoice
        id={`${id}-folders`}
        label={t("task:checkoutOptions.folders")}
        selected={props.selectedFolders}
        onChange={props.setSelectedFolders}
        defaultLabel={t("task:checkoutOptions.allFolders")}
        selectedLabel={t("task:checkoutOptions.selectedFolders")}
        disabled={!capabilities?.sparse}
        testId="repository-options-folders"
      />
      {props.selectedFolders && (
        <div className="space-y-2">
          <label htmlFor={`${id}-directories`} className="text-xs text-muted-foreground">
            {t("task:checkoutOptions.directories")}
          </label>
          <Textarea
            id={`${id}-directories`}
            value={props.directories}
            onChange={(event) => props.setDirectories(event.target.value)}
            rows={4}
            className="resize-none text-base md:text-xs"
            aria-invalid={props.invalid}
            aria-describedby={`${id}-help`}
            data-testid="repository-options-directories"
          />
          <p id={`${id}-help`} className="text-xs text-muted-foreground">
            {t("task:checkoutOptions.foldersHelp")}
          </p>
          {props.invalid && (
            <p role="alert" className="text-xs text-destructive">
              {t("task:checkoutOptions.invalidDirectories")}
              {props.errorLines?.map((line) => (
                <span key={line} className="block">
                  {t("task:checkoutOptions.invalidLine", { line })}
                </span>
              ))}
            </p>
          )}
        </div>
      )}
      {(!capabilities || capabilities.reason) && (
        <p role="status" className="text-xs text-muted-foreground">
          {checkoutSupportMessage(props, t)}
        </p>
      )}
      {props.failed && (
        <Button
          type="button"
          variant="outline"
          className="h-7 cursor-pointer [@media(pointer:coarse)]:h-11"
          onClick={props.retry}
        >
          {t("task:retry")}
        </Button>
      )}
    </div>
  );
}

function CheckoutChoice({
  id,
  label,
  selected,
  onChange,
  defaultLabel,
  selectedLabel,
  disabled,
  testId,
}: {
  id: string;
  label: string;
  selected: boolean;
  onChange: (value: boolean) => void;
  defaultLabel: string;
  selectedLabel: string;
  disabled: boolean;
  testId: string;
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      <label htmlFor={id} className="text-xs text-muted-foreground">
        {label}
      </label>
      <Select
        value={selected ? "selected" : "default"}
        onValueChange={(value) => onChange(value === "selected")}
      >
        <SelectTrigger
          id={id}
          data-testid={testId}
          className="w-36 border-border/60 bg-muted/30 hover:bg-muted/60"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="default" className="[@media(pointer:coarse)]:min-h-11">
            {defaultLabel}
          </SelectItem>
          <SelectItem
            value="selected"
            disabled={disabled}
            className="[@media(pointer:coarse)]:min-h-11"
          >
            {selectedLabel}
          </SelectItem>
        </SelectContent>
      </Select>
    </div>
  );
}

function checkoutSupportMessage(props: Props, t: TFunction) {
  if (props.failed) return t("task:checkoutOptions.unavailable");
  if (!props.capabilities) return t("task:checkoutOptions.checking");
  if (props.capabilities.reason === "credentials_unsupported")
    return t("task:checkoutOptions.credentialsUnsupported");
  if (props.capabilities.reason === "executor_required")
    return t("task:checkoutOptions.executorRequired");
  if (props.capabilities.reason === "provider_unsupported")
    return t("task:checkoutOptions.providerUnsupported");
  return t("task:checkoutOptions.preparationUnsupported");
}
