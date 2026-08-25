# Kandev Specifications

This directory contains the durable product and system specifications for
Kandev. A specification describes product intent, required behavior, or the
technical design of a system.

New specifications use this structure:

```text
docs/specs/
├── product/
└── <system>/
    ├── README.md
    ├── glossary.md
    ├── requirements/
    └── system-design/
```

Read the [specification guide](guide/README.md) before you create or change a
specification. Use the files in [templates](templates) for new documents.

The [specification catalog](INDEX.md) is the entry point for the completed
system-oriented layout. Each system README names its authoritative requirements
and system designs.

Plans and work orders are in [`docs/plans`](../plans). Architecture decisions
are in [`docs/decisions`](../decisions). Public user documentation is in
[`docs/public`](../public).
