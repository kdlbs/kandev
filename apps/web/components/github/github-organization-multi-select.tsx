"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Spinner } from "@kandev/ui/spinner";
import { cn } from "@/lib/utils";
import { useGitHubOrgs, type UseGitHubOrgsResult } from "@/hooks/domains/github/use-github-orgs";
import { AlreadyAddedMarker, isPlainEnter, PickerSearchRow } from "@/components/picker-controls";
import { parseGitHubLogin } from "@/lib/github/repository-input";
import { SelectionAddButton, SelectionChip } from "./selection-chip";

export type GitHubOrganizationMultiSelectProps = {
  workspaceId: string | undefined;
  /** Selected GitHub user or organization logins. */
  selected: string[];
  onChange: (logins: string[]) => void;
  /** When set, only organizations it accepts can be listed or added. */
  isOrgAllowed?: (login: string) => boolean;
  disabled?: boolean;
};

const sameLogin = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();

/**
 * Selected GitHub organizations as removable chips, plus an "Add organization"
 * control that lists the authenticated account and its organizations.
 */
export function GitHubOrganizationMultiSelect({
  workspaceId,
  selected,
  onChange,
  isOrgAllowed,
  disabled = false,
}: GitHubOrganizationMultiSelectProps) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1.5">
      {selected.map((login) => (
        <SelectionChip
          key={login.toLowerCase()}
          label={login}
          removeLabel={t("github:removeOrganization", { organization: login })}
          testId="github-organization-chip"
          disabled={disabled}
          onRemove={() => onChange(selected.filter((entry) => !sameLogin(entry, login)))}
        />
      ))}
      <AddGitHubOrganization
        workspaceId={workspaceId}
        selected={selected}
        onChange={onChange}
        isOrgAllowed={isOrgAllowed}
        disabled={disabled}
      />
    </div>
  );
}

function AddGitHubOrganization({
  workspaceId,
  selected,
  onChange,
  isOrgAllowed,
  disabled,
}: Omit<GitHubOrganizationMultiSelectProps, "disabled"> & { disabled: boolean }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const orgs = useGitHubOrgs(workspaceId);
  const isSelected = (login: string) => selected.some((entry) => sameLogin(entry, login));
  const add = (login: string) => {
    if (!isSelected(login)) onChange([...selected, login]);
    setOpen(false);
  };
  return (
    <Popover open={open && !disabled} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <SelectionAddButton
          label={t("github:addOrganization")}
          testId="github-organization-add"
          disabled={disabled}
        />
      </PopoverTrigger>
      <PopoverContent
        className="w-[320px] max-w-[calc(100vw-2rem)] max-h-[min(420px,calc(100vh-12rem))] overflow-hidden p-0"
        align="start"
        portal={false}
        data-testid="github-organization-picker"
      >
        <OrganizationPickerPanel
          orgs={orgs}
          isSelected={isSelected}
          isOrgAllowed={isOrgAllowed}
          onAdd={add}
        />
      </PopoverContent>
    </Popover>
  );
}

function OrganizationPickerPanel({
  orgs,
  isSelected,
  isOrgAllowed,
  onAdd,
}: {
  orgs: UseGitHubOrgsResult;
  isSelected: (login: string) => boolean;
  isOrgAllowed?: (login: string) => boolean;
  onAdd: (login: string) => void;
}) {
  const { t } = useTranslation();
  const [value, setValue] = useState("");
  const [submitError, setSubmitError] = useState<string | null>(null);
  const needle = value.trim().toLowerCase();
  const visible = orgs.orgs
    .map((org) => org.login)
    .filter((login) => !isOrgAllowed || isOrgAllowed(login))
    .filter((login) => login.toLowerCase().includes(needle));
  const typedLogin = parseGitHubLogin(value);
  const onKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (!isPlainEnter(event) || !typedLogin) return;
    event.preventDefault();
    if (isOrgAllowed && !isOrgAllowed(typedLogin)) {
      setSubmitError(t("github:organizationOutsideWorkspaceScope"));
      return;
    }
    onAdd(typedLogin);
  };
  return (
    <div className="flex flex-col">
      <PickerSearchRow
        value={value}
        onChange={(next) => {
          setValue(next);
          setSubmitError(null);
        }}
        onKeyDown={onKeyDown}
        placeholder={t("github:searchOrganizationsOrEnterLogin")}
        invalid={Boolean(submitError)}
        testId="github-organization-input"
        refresh={{
          onRefresh: orgs.refresh,
          refreshing: orgs.loading,
          label: t("github:refreshOrganizations"),
          testId: "github-organization-refresh",
        }}
      />
      {typedLogin ? (
        <div className="px-2 pt-1 text-xs text-muted-foreground">
          {t("github:pressEnterToAddOrganization")}
        </div>
      ) : null}
      <div className="h-56 max-h-[calc(100vh-16rem)] overflow-y-auto p-1">
        <OrganizationListStatus orgs={orgs} empty={visible.length === 0} error={submitError} />
        {visible.map((login) => (
          <button
            key={login}
            type="button"
            onClick={() => onAdd(login)}
            data-testid="github-organization-option"
            className={cn(
              "flex min-h-11 sm:min-h-8 w-full items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-xs",
              "hover:bg-muted cursor-pointer text-left",
            )}
          >
            <span className="truncate">{login}</span>
            {isSelected(login) ? <AlreadyAddedMarker /> : null}
          </button>
        ))}
      </div>
    </div>
  );
}

function OrganizationListStatus({
  orgs,
  empty,
  error,
}: {
  orgs: UseGitHubOrgsResult;
  empty: boolean;
  error: string | null;
}) {
  const { t } = useTranslation();
  if (error) {
    return (
      <div role="alert" className="px-2 py-3 text-xs text-destructive">
        {error}
      </div>
    );
  }
  if (orgs.loading && empty) {
    return (
      <div className="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground">
        <Spinner className="size-3" />
        <span>{t("github:loadingOrganizations")}</span>
      </div>
    );
  }
  if (orgs.error) {
    return (
      <div className="px-2 py-3 text-xs text-destructive">
        {t("github:couldNotLoadOrganizations", { message: orgs.error.message })}
      </div>
    );
  }
  if (empty && !orgs.loading) {
    return (
      <div className="px-2 py-3 text-xs text-muted-foreground">
        {t("github:noOrganizationsFound")}
      </div>
    );
  }
  return null;
}
