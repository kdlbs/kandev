# Feature flags and rollout

Read this reference before collecting rollout evidence or writing `feature_flags`.
Every new walkthrough needs this section, including PRs without feature flags.
The section follows impact and precedes architecture and code.

## Evidence

1. Identify each feature behavior that the PR adds or changes.
2. Trace its gates from registration through backend handlers and frontend entry points.
3. Check each flag's identity, defaults, activation method, overrides, and restart requirement.
4. Compare the base experience with the head experience when all relevant flags are off.
5. Inspect controls, navigation, settings, messages, empty states, errors, and phone layouts outside the gates.
6. Check tests for both enabled and disabled paths.

For Kandev, inspect the runtime registry, `profiles.yaml`, config readers, boot payload, and frontend flag consumers.
Include existing flags that cover the PR, not only flags that the PR adds.
A registered flag does not prove that the feature uses it.
A disabled backend endpoint does not prove that its UI entry point disappears.
Settings controls for a new flag can change the UX even when the feature is off.

Separate the feature's gated behavior from incidental UX changes outside the gates.
Report every ungated feature behavior in `summary`.
Report every visible flag-off change in `off_ux.items`, including flag-management controls.
Keep those changes in `impact.ux` as well. The flag-off table identifies which changes remain without activation.
Do not claim unchanged UX without evidence from the disabled paths.
Use `unknown` when prepared files or tests cannot establish the result.
When evidence gaps coexist with known changes, describe the gaps in `summary` or `off_ux.note`.
Never treat a flag as an approval verdict or automatically reduce the risk score.

## JSON shape

`feature_flags` requires `coverage`, `summary`, `flags`, and `off_ux`.
`summary` is a non-empty explanation of the coverage and any ungated behavior.

| Coverage | Meaning |
| --- | --- |
| `full` | Flags gate all feature behavior. Incidental UX changes can still appear in `off_ux`. |
| `partial` | Flags gate some feature behavior. The summary names behavior that ships without flags. |
| `none` | Feature behavior ships without flags. |
| `not_applicable` | The PR changes no feature behavior, such as a documentation-only PR. |
| `unknown` | Evidence cannot establish flag coverage. The summary explains the gap. |

`flags` has one row per relevant flag. Each row requires these non-empty strings:

| Field | Content |
| --- | --- |
| `key` | Exact flag identity. Include the environment alias when relevant. |
| `change` | `new`, `existing`, `changed`, or `removed`, relative to the comparison base. |
| `default` | Defaults per shipped profile, activation method, and restart requirement. State unknown values explicitly. |
| `enabled` | What users can do with the flag on. |
| `disabled` | What remains with the flag off, including fallback behavior and work outside the gate. |
| `file` | Repository-relative changed file that establishes the flag or its gate. |

`full` and `partial` require at least one active flag row.
For `none` or `not_applicable`, use an empty array unless the PR removes a flag.
For removed flags, explain the now-unconditional behavior and the removal of the on/off control.
For existing flags, link the changed gate and name the supporting registry or profile evidence in `default`.

`off_ux` uses the same row fields as `impact.ux`: `surface`, `before`, `after`, and `file`.
Its status is `changed`, `none`, or `unknown`.
`changed` requires at least one row. `none` and `unknown` require an empty array and a non-empty `note`.
For `none`, explain the evidence that the disabled path preserves the base UX.
For `unknown`, explain what evidence is missing.
When the PR has no active flag, describe the UX that ships without activation.

```json
{
  "feature_flags": {
    "coverage": "full",
    "summary": "The flag gates the new picker. The flag-management control appears even with the flag off.",
    "flags": [{
      "key": "new_picker / KANDEV_NEW_PICKER",
      "change": "new",
      "default": "prod: off; dev: off; e2e: off. Enable in Feature Toggles, then restart.",
      "enabled": "Users select an item in the new picker.",
      "disabled": "Users keep the existing picker. The flag-management control remains visible.",
      "file": "path/to/changed/picker.tsx"
    }],
    "off_ux": {
      "status": "changed",
      "items": [{
        "surface": "Settings > System > Feature Toggles",
        "before": "No control exists for the new picker.",
        "after": "The new picker toggle appears, even with the flag off.",
        "file": "path/to/changed/registry.go"
      }]
    }
  }
}
```

This synthetic example is not evidence about any real flag or PR.

## Validation

Check coverage and flag-off UX independently. Check defaults against the actual registry and profiles.
Check source links, unknown states, and removed flags.
Use the renderer and browser commands in [impact.md](impact.md#validation).
The phone view uses labeled cards with every flag field and flag-off UX row.
