---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-WEB-ASSET-DELIVERY-001
---

# Web asset delivery system design

## Ownership and mapping

The Vite build produces assets. `apps/backend/internal/webapp/handler.go`
serves them from its existing `fs.FS`. This design implements
`REQ-PLATFORM-WEB-ASSET-DELIVERY-001` within those owners. It introduces no
configuration, database state, authorization change, or runtime compression
cache. Component-level lazy loading remains in the
[UI lifecycle design](../../ui/system-design/session-refresh-efficiency.md).

## Build representations

Add a small build-only Vite plugin backed by a separately testable Node helper
in `apps/web/scripts/`. After output is written, generate deterministic `.gz`
siblings for emitted `.js` and `.css` files of at least 1 KiB. Use Node's
standard gzip implementation. Retain a sibling only when it is smaller than
the original. Preserve originals and exclude HTML, source maps, and media.
Use the resolved output directory, including custom `outDir` builds.

Keep the step in the Vite build lifecycle so `build`, `build:vite`, E2E, and
browser-demo builds do not diverge. Await all writes before build completion.
Inspect the native resource-copy and release archive paths; they must carry
siblings with the original assets. A smoke check must load a packaged asset
tree through the production handler. Missing siblings retain identity delivery.

## Representation selection

Keep the current normalized path and filesystem checks. Establish that the
original asset exists before considering its compressed sibling. Negotiate
gzip and identity using `Accept-Encoding`, including explicit zero quality,
wildcards, case-insensitive coding names, and client preferences. Choose gzip
on equal acceptable preference; choose identity for an absent or empty header.
Return 406 if neither available representation is acceptable. Do not treat a
substring containing `gzip` as consent. Use the HTTP semantics in
[RFC 9110 sections 8.4, 12.5.3, and 12.5.5](https://www.rfc-editor.org/rfc/rfc9110.html#name-accept-encoding).

Append `Accept-Encoding` to `Vary` on negotiable asset responses, including
identity and conditional responses. Use the original filename for media type
and the selected file for length, modification time, and response bytes. Set
`Content-Encoding: gzip` only for a selected gzip representation. Preserve
immutable asset caching. Do not add content hashes at request time.

Reuse `http.ServeContent` over the selected representation. Range offsets and
HEAD lengths refer to that representation.
For gzip assets, ignore multiple ranges and send the complete representation;
a multipart envelope cannot carry the asset's gzip content encoding.
Test the Go version's actual conditional and range behavior, including 304 and
416; do not leave an encoding header on an unencoded error body. Do not add a
strong ETag shared by identity
and gzip. If a validator is introduced, it must identify the representation.
The existing non-seekable filesystem path needs the same representation rules.

The private `RenderShell` response remains `no-store`. API and WebSocket
handlers are outside this code path. Development Vite serving retains its
current behavior. No proxy configuration is required for correctness.

## Verification and failure behavior

Helper tests verify deterministic output, decompression equality, thresholds,
smaller-only emission, repeated builds, and alternate output directories.
Handler tests cover gzip, identity, missing siblings, quality values, wildcard
exclusion, MIME type, Vary merging, GET/HEAD, conditional responses, ranges,
missing paths, and seekable/non-seekable filesystems. Build failures must fail
the build; an incomplete compressed file must not be shipped.

Measure encoded response bytes separately from decoded resource size on cold
loads. Compression cannot be credited with reducing JavaScript execution or
warm task-switch time. Existing desktop and phone routes must load and remain
interactive from the packaged production tree.

## Implementation plans

- [Asset delivery extraction](../../../plans/asset-delivery-extraction/plan.md)
