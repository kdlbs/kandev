import { describe, expect, it } from "vitest";
import { resolveAuthenticationRecovery } from "./session-authentication-recovery";

const CURSOR_LOGIN = "cursor_login";
const AUTHENTICATION_REQUIRED = "authentication_required";
const CURSOR_LOGIN_NAME = "Cursor login";

describe("session authentication recovery", () => {
  it("uses the durable typed cause when transcript metadata is absent", () => {
    expect(
      resolveAuthenticationRecovery([{ operation: "start", code: AUTHENTICATION_REQUIRED }]),
    ).toEqual({ required: true, methods: [] });
  });

  it("keeps bounded advertised methods from recovery metadata", () => {
    const recovery = resolveAuthenticationRecovery(
      [{ operation: "start", code: AUTHENTICATION_REQUIRED }],
      {
        is_auth_error: true,
        auth_methods: [
          { id: CURSOR_LOGIN, name: CURSOR_LOGIN_NAME, description: "Sign in with Cursor" },
          {
            id: "terminal_login",
            name: "Terminal login",
            terminal_auth: { command: "provider", args: ["login"] },
            meta: { credential: "private-value" },
          },
        ],
      },
    );

    expect(recovery).toEqual({
      required: true,
      methods: [
        { id: CURSOR_LOGIN, name: CURSOR_LOGIN_NAME, description: "Sign in with Cursor" },
        {
          id: "terminal_login",
          name: "Terminal login",
          terminal_auth: { command: "provider", args: ["login"] },
        },
      ],
    });
  });

  it("does not infer authentication from a flag that contradicts typed causes", () => {
    expect(
      resolveAuthenticationRecovery([{ operation: "start", code: "permission_denied" }], {
        is_auth_error: true,
        auth_methods: [{ id: CURSOR_LOGIN, name: CURSOR_LOGIN_NAME }],
      }),
    ).toEqual({ required: false, methods: [] });
  });

  it("does not turn a protocol method ID into a terminal command", () => {
    const recovery = resolveAuthenticationRecovery(undefined, {
      is_auth_error: true,
      auth_methods: [{ id: CURSOR_LOGIN, name: CURSOR_LOGIN_NAME }],
    });

    expect(recovery.methods[0]).toEqual({ id: CURSOR_LOGIN, name: CURSOR_LOGIN_NAME });
  });

  it("drops malformed and oversized method entries", () => {
    const recovery = resolveAuthenticationRecovery(undefined, {
      is_auth_error: true,
      auth_methods: [
        { id: "", name: "Missing ID" },
        { id: "unsafe", name: "Unsafe", terminal_auth: { command: "bad command; echo leak" } },
        { id: "valid", name: "Valid" },
        { id: "oversized", name: "x".repeat(257) },
      ],
    });

    expect(recovery).toEqual({
      required: true,
      methods: [
        { id: "unsafe", name: "Unsafe" },
        { id: "valid", name: "Valid" },
        { id: "oversized", name: "oversized" },
      ],
    });
    expect(recovery.methods[0].terminal_auth).toBeUndefined();
  });
});
