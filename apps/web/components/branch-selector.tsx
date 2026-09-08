"use client";

import { memo } from "react";

import { BranchRefreshButton } from "./branch-refresh-button";
import { Combobox } from "./combobox";
import { t } from "@/lib/i18n";
import { scoreBranch } from "@/lib/utils/branch-filter";

const CURSOR_POINTER_CLASS = "cursor-pointer";

export type BranchOption = {
  value: string;
  label: string;
  keywords?: string[];
  renderLabel?: () => React.ReactNode;
  group?: string;
  groupLabel?: string;
};

export type BranchSelectorProps = {
  options: BranchOption[];
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  searchPlaceholder: string;
  emptyMessage: string;
  triggerClassName?: string;
  onRefresh?: () => void;
  refreshing?: boolean;
  fetchedAt?: string;
  fetchError?: string;
  loading?: boolean;
  ariaLabel?: string;
  testId?: string;
  dropdownTestId?: string;
  dropdownLabel?: string;
  touchTarget?: boolean;
  triggerId?: string;
};

export const BranchSelector = memo(function BranchSelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  searchPlaceholder,
  emptyMessage,
  triggerClassName,
  onRefresh,
  refreshing,
  fetchedAt,
  fetchError,
  loading,
  ariaLabel,
  testId = "branch-selector",
  dropdownTestId,
  dropdownLabel = t("task:baseBranch2"),
  touchTarget = false,
  triggerId,
}: BranchSelectorProps) {
  const headerAction = onRefresh ? (
    <BranchRefreshButton
      onRefresh={onRefresh}
      refreshing={refreshing}
      fetchedAt={fetchedAt}
      fetchError={fetchError}
      touchTarget={touchTarget}
    />
  ) : undefined;
  return (
    <Combobox
      options={options}
      value={value}
      onValueChange={onValueChange}
      placeholder={placeholder}
      searchPlaceholder={searchPlaceholder}
      emptyMessage={emptyMessage}
      disabled={disabled}
      ariaLabel={ariaLabel}
      dropdownLabel={dropdownLabel}
      className={disabled ? undefined : CURSOR_POINTER_CLASS}
      triggerClassName={triggerClassName}
      testId={testId}
      dropdownTestId={dropdownTestId}
      filter={scoreBranch}
      headerAction={headerAction}
      loading={loading}
      touchTarget={touchTarget}
      triggerId={triggerId}
    />
  );
});
