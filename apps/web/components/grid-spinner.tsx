"use client";

type GridSpinnerProps = {
  className?: string;
};

export function GridSpinner({ className }: GridSpinnerProps) {
  return (
    <span className={`spinner-grid ${className ?? ""}`} role="status" aria-label="Loading">
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
