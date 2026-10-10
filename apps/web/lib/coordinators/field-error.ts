import { ApiError } from "@/lib/api/client";

export type CoordinatorErrorField = "name" | "agent_profile_id" | "executor_profile_id" | "context";

export type CoordinatorFieldError = {
  field: CoordinatorErrorField | null;
  message: string;
};

function isCoordinatorErrorField(value: unknown): value is CoordinatorErrorField {
  return (
    value === "name" ||
    value === "agent_profile_id" ||
    value === "executor_profile_id" ||
    value === "context"
  );
}

// Mirrors internal/coordinator/dto.go's `{"error","field"}` 400 body (B11). A
// field absent from the known set (or missing entirely) is a form-level error.
export function coordinatorFieldError(error: unknown): CoordinatorFieldError | null {
  if (!(error instanceof ApiError) || error.status !== 400) return null;
  const body = error.body;
  const record = body && typeof body === "object" ? (body as Record<string, unknown>) : null;
  const message = typeof record?.error === "string" ? record.error : error.message;
  const field = isCoordinatorErrorField(record?.field) ? record.field : null;
  return { field, message };
}
