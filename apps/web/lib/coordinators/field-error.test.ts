import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import { coordinatorFieldError } from "./field-error";

const BAD_REQUEST = "bad request";

describe("coordinatorFieldError", () => {
  it("returns null for a non-ApiError", () => {
    expect(coordinatorFieldError(new Error("boom"))).toBeNull();
  });

  it("returns null for an ApiError that is not a 400", () => {
    expect(coordinatorFieldError(new ApiError("not found", 404, {}))).toBeNull();
  });

  it("extracts the field name from a 400 field-error body (B11)", () => {
    const error = new ApiError(BAD_REQUEST, 400, {
      error: "name too long",
      field: "name",
    });
    expect(coordinatorFieldError(error)).toEqual({ field: "name", message: "name too long" });
  });

  it("reports a form-level error when the 400 body has no field (B11)", () => {
    const error = new ApiError(BAD_REQUEST, 400, { error: "something went wrong" });
    expect(coordinatorFieldError(error)).toEqual({
      field: null,
      message: "something went wrong",
    });
  });

  it("falls back to the error message when the body is not the expected shape", () => {
    const error = new ApiError(BAD_REQUEST, 400, null);
    expect(coordinatorFieldError(error)).toEqual({ field: null, message: BAD_REQUEST });
  });
});
