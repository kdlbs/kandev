# ADR-2026-08-18-session-mcp-tool-definition-details: Bound MCP Tool Definition Details to Current Sessions

**Status:** accepted
**Date:** 2026-08-18
**Area:** backend, agentctl, frontend, protocol, security

## Context

The MCP explorer stores tool names and descriptions for Kandev's current
session server. This data does not show a tool's arguments or its relative
context size.

MCP defines tool input schemas but does not define a tokenizer. Agent providers
also transform MCP tools into different model inputs. As a result, Kandev
cannot calculate one exact context cost for every agent and model.

## Decision

Kandev stores bounded input schemas with the current Kandev tool catalog. Each
schema comes from the actual `tools/list` response that Kandev served to the
agent. Kandev does not store output schemas, annotations, `_meta`, invocation
arguments, or results.

Each stored input schema has a 64 KiB limit. The current catalog has a 512 KiB
combined schema limit. Kandev omits a schema that exceeds either limit and
marks that tool as truncated. Kandev keeps the tool name, description, and
token estimate when those fields are available.

Kandev calculates a deterministic token estimate from the compact JSON for one
complete MCP tool definition. The calculation uses the `o200k_base` encoding.
It includes fields sent in that tool definition, but it excludes the enclosing
`tools/list` response and provider-specific wrappers.

The wire contract identifies the estimate method as
`o200k_base:mcp-tool-json-v1`. The UI shows `~N tokens` and explains that the
value is an estimate. Kandev does not use a character or byte heuristic when
the tokenizer is unavailable.

The backend uses an offline tokenizer. The tokenizer must not download data at
runtime or send tool definitions to a provider. The initial implementation
uses `github.com/tiktoken-go/tokenizer` with only the `o200k_base` codec at the
call site.

The catalog remains session owned and current-attempt only. Kandev does not
connect to third-party MCP servers to collect schemas or token estimates.
Historical attempts keep the total tool count but no tool definitions.

## Consequences

Users can inspect Kandev tool arguments and compare definition sizes without a
network request. The estimate is stable for the same MCP tool JSON and encoder.

The estimate is not the agent's billable token count. A provider can use a
different tokenizer, wrapper, cache, or tool-loading strategy.

Input schemas increase session metadata. The per-schema and catalog limits
bound this increase. A large schema can be unavailable in the UI even when the
agent received the complete schema.

## Alternatives Considered

### Show an exact count for the selected agent model

Rejected. Kandev does not own a common provider API that returns per-tool
counts. Provider token-count APIs also include provider wrappers and often
return only a request total.

### Use character or byte counts as tokens

Rejected. Tokenizers split the same text differently. A character ratio can
mislead users for JSON, identifiers, and non-English text.

### Call a provider token-count API

Rejected. This path needs provider credentials, adds latency and rate limits,
and still does not support every agent. It can also send tool definitions to a
new external service.

### Store complete MCP tool objects

Rejected. Output schemas, annotations, and `_meta` are not needed for the
requested argument view. They increase metadata and the provider-content
surface.

### Add an on-demand schema endpoint

Rejected. A separate live endpoint produces different results after reload or
disconnection. The current session evidence path already owns the observed
catalog.
