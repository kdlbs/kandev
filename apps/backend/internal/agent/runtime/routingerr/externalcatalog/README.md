# External provider-error catalog

Format reference for the operator catalog named by the `providerErrors.catalogFile`
startup setting (`KANDEV_PROVIDER_ERRORS_CATALOG_FILE`). Requirement and design:
`docs/specs/agents/{requirements,system-design}/provider-error-signatures.md`
(REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-003).

## Behavior

- No configured path: no file I/O, built-in rules only. A relative path
  resolves against the directory of the selected config file.
- A configured file is read once, decoded, validated, and compiled into an
  immutable ordered rule set. Any error rejects the whole file; built-in rules
  stay in effect and no rule from the file is applied.
- External rules add evidence; they never replace built-in rules. Built-in
  structured, provider, provider-neutral, and runtime rules are evaluated
  first. External rules run before the opt-in keyword/context fallback and the
  phase default. Within the file, the first matching rule wins.
- Rules that use `terms` or `match.context` only run for an eligible provider
  diagnostic: the opt-in context classifier is enabled and the text is a
  provider failure rather than agent prose or a quote. A catalog file does not
  enable that classifier.
- Load errors expose a stable reason, rule index, rule id, and field name.
  They never include file content, parser messages, or the file path.

## Document

JSON object or YAML mapping. Extensions: `.json`, `.yaml`, `.yml`. Unknown
fields, trailing data, and extra YAML documents are rejected.

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `version` | integer | yes | Must be `1`. |
| `rules` | list | no | Ordered rules. An empty list adds nothing. |
| `rules[].id` | string | yes | Unique; `[A-Za-z0-9][A-Za-z0-9._-]{0,127}`. |
| `rules[].match` | mapping | yes | All present conditions must hold (AND). |
| `rules[].result` | mapping | yes | Classification produced on a match. |

### `match`

At least one of `pattern` or `terms` is required. In `providers`, `phases`,
`httpStatus`, and the `context` lists, values are alternatives (OR). `terms`
is different: at least `minTerms` distinct terms must match.

| Field | Type | Meaning |
| --- | --- | --- |
| `pattern` | Go RE2 regexp | Matched against the sanitized diagnostic. |
| `terms` | list of strings | Normalized (lowercase, collapsed spaces) evidence terms, matched with the classifier's keyword normalization. |
| `minTerms` | integer | Required with `terms`. Distinct terms that must match; at least 2 when `pattern` is absent. |
| `providers` | list of strings | Provider ids. |
| `phases` | list of strings | Classifier phases: `process_start`, `session_init`, `prompt_send`, `streaming`, and the other values the classifier accepts. |
| `httpStatus` | list of integers | Structured HTTP status, 100-599. |
| `context` | mapping | Typed invocation evidence, below. |

`context` fields: `tiers`, `accounts`, `models`, `paths` (lists of exact
values; a missing observed value never matches), `freeDaily` and
`emptyOutput` (booleans), `minConsecutiveFailures`, `minOtherPaths`,
`minOtherModels` (non-negative integers).

### `result`

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `code` | string | yes | One of `auth_required`, `missing_credentials`, `subscription_required`, `quota_limited`, `rate_limited`, `network_unavailable`, `provider_unavailable`, `provider_overloaded`, `model_capacity`, `model_unavailable`, `provider_not_configured`, `provider_resource_exhausted`. Codes that hold until an owner releases them are not accepted. |
| `confidence` | string | yes | `high`, `medium`, or `low`. |
| `scope` | string | no | `path_model` (default), `model`, or `account`. Widening applies only to an eligible diagnostic that carries the model or account id; otherwise `path_model`. |
| `backoff` | Go duration | no | Positive duration, for example `10m`. Omitted keeps the classifier default. |
| `reset` | string | no | `inherit` (default: observed reset hints as today), `free_daily` (requires `match.context.freeDaily: true`), or `until_model_change` (requires `code: model_unavailable` and `httpStatus: [404]`). |

`scope`, `backoff`, and `reset` take effect only while the keyword and context
classification (`features.providerKeywordContext`) is enabled for a provider
diagnostic; otherwise only `code` and `confidence` apply. Account and tier
context is known only when Kandev refines an already classified failure, so a
rule that depends on them can narrow or widen scope but cannot change the code.

Class, retryability, fallback permission, and recovery actions are derived by
the classifier from `code`; a catalog cannot set them.

## Errors

| Reason | Cause |
| --- | --- |
| `unreadable` | The configured file cannot be read. |
| `unsupported_extension` | Not `.json`, `.yaml`, or `.yml`. |
| `malformed` | Decode failure, trailing data, or more than one YAML document. |
| `unknown_field` | A field outside this schema. |
| `unsupported_version` | `version` other than `1`. |
| `missing_field` | A required field is absent. |
| `invalid_value` | A value outside its allowed set or contract. |
| `duplicate_id` | Two rules share an id. |
| `invalid_pattern` | `pattern` is not a valid RE2 expression. |
| `invalid_duration` | `backoff` is not a positive Go duration. |

## Examples

`examples/catalog.example.yaml` and `examples/catalog.example.json` are
equivalent documents covering a provider pattern, keyword terms with context,
a free daily reset, and a missing-model reset.
