import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  buildExecutorProfileValues,
  parseExecutorProfileSchema,
  serializeExecutorProfileValues,
  validateExecutorProfileValues,
} from "@/lib/plugins/executor-profile-schema";
import {
  executorProfileUnavailableReason,
  isExecutorProfileProviderAvailable,
} from "@/lib/executor-provider-display";
import { PluginExecutorProfileFields } from "./plugin-executor-profile-fields";

const schema = {
  type: "object",
  required: ["region", "credential"],
  properties: {
    region: { type: "string", title: "Region", enum: ["eu-west-1", "us-east-1"] },
    workers: { type: "integer", title: "Workers", minimum: 1, maximum: 10 },
    credential: { type: "string", title: "Credential", secret: true },
  },
} satisfies Record<string, unknown>;

afterEach(() => cleanup());

describe("plugin executor profile", () => {
  it("parses the scalar schema and binds required and range errors to fields", () => {
    const { fields, supported } = parseExecutorProfileSchema(schema);
    expect(supported).toBe(true);
    expect(
      fields.map(({ name, type, required, secret }) => ({ name, type, required, secret })),
    ).toEqual([
      { name: "region", type: "string", required: true, secret: false },
      { name: "workers", type: "integer", required: false, secret: false },
      { name: "credential", type: "string", required: true, secret: true },
    ]);
    expect(
      validateExecutorProfileValues(fields, {
        region: "ap-south-1",
        workers: "11",
        credential: "",
      }),
    ).toEqual({
      region: { code: "enum" },
      workers: { code: "maximum", value: 10 },
      credential: { code: "required" },
    });
  });

  it("never loads a secret value, keeps a configured secret, and submits a replacement only when typed", () => {
    const { fields } = parseExecutorProfileSchema(schema);
    const values = buildExecutorProfileValues(fields, { region: "eu-west-1" });
    expect(values.credential).toBe("");
    expect(validateExecutorProfileValues(fields, values, { credential: true })).toEqual({});
    expect(serializeExecutorProfileValues(fields, values)).toEqual({
      region: "eu-west-1",
      workers: "",
    });
    expect(serializeExecutorProfileValues(fields, { ...values, credential: "new-token" })).toEqual({
      region: "eu-west-1",
      workers: "",
      credential: "new-token",
    });
  });

  it("marks a secret for clear without exposing it in the field", () => {
    const onClearSecret = vi.fn();
    const { fields } = parseExecutorProfileSchema(schema);
    render(
      <PluginExecutorProfileFields
        fields={fields}
        values={{ region: "eu-west-1", workers: "2", credential: "" }}
        configuredSecrets={{ credential: true }}
        onValueChange={vi.fn()}
        onClearSecret={onClearSecret}
      />,
    );

    expect(screen.getByTestId("executor-profile-secret-configured-credential")).not.toBeNull();
    expect((screen.getByLabelText(/Credential/) as HTMLInputElement).value).toBe("");
    fireEvent.click(screen.getByTestId("executor-profile-secret-clear-credential"));
    expect(onClearSecret).toHaveBeenCalledWith("credential");
  });

  it("keeps an unavailable provider choice identifiable and disabled", () => {
    const t = (key: string) => key;
    const profile = {
      id: "profile-1",
      executor_id: "executor-1",
      executor_type: "plugin_remote" as const,
      name: "Retained profile",
      provider: {
        executor_id: "executor-1",
        identity: "plugin:test:provider",
        plugin_id: "test",
        installation_id: "install-1",
        key: "provider",
        contract_version: 1,
        display_name: "Test provider",
        description: "Provider fixture",
        profile_schema: schema,
        capabilities: {
          terminal: true,
          files: true,
          git: true,
          embedded_editor: false,
          preview: false,
          reattach: true,
          retention: "bounded",
          maximum_lifetime_seconds: 28800,
        },
        available: false,
        availability_cause: "plugin_unavailable",
      },
      prepare_script: "",
      cleanup_script: "",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };
    expect(executorProfileUnavailableReason(profile, t)).toBe(
      "executors:providerPluginUnavailable",
    );
    expect(isExecutorProfileProviderAvailable(profile)).toBe(false);
  });
});
