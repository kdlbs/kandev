"use client";
import { useTranslation } from "react-i18next";

type GridSpinnerProps = {
  className?: string;
};

export function GridSpinner({ className }: GridSpinnerProps) {
  const { t } = useTranslation();
  return (
    <span
      className={`spinner-grid ${className ?? ""}`}
      role="status"
      aria-label={t("common:loadingIndicatorLabel")}
    >
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
    </span>
  );
}
