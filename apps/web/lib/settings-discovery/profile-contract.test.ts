import { describe, expect, it } from "vitest";
import {
  PROFILE_EDITABLE_FIELD_PATHS,
  profileContractCovers,
  profileContractField,
  profileContractFields,
} from "./profile-contract";

describe("profile settings contract adapter", () => {
  it("covers every editable profile field with generated metadata", () => {
    const fields = profileContractFields();

    expect(fields.map((field) => field.field_path)).toEqual([...PROFILE_EDITABLE_FIELD_PATHS]);
    expect(fields.every((field) => field.support === "supported" && field.writable)).toBe(true);
  });

  it("keeps replacement and sensitive profile fields discoverable", () => {
    expect(profileContractField("agent_profile", "config_options")?.replacement).toBe(true);
    expect(profileContractField("agent_profile", "cli_flags")?.replacement).toBe(true);
    expect(profileContractField("agent_profile", "env_vars")?.sensitive).toBe(true);
    expect(profileContractField("agent_profile_mcp", "servers")?.sensitive).toBe(true);
  });

  it("does not turn provider-defined dynamic keys into catalog fields", () => {
    expect(profileContractCovers("provider_specific_option")).toBe(false);
    expect(profileContractField("agent_profile", "provider_specific_option")).toBeUndefined();
  });
});
