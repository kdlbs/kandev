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

The [specification catalog entry page](INDEX.md) explains how to query current
documents. Run the catalog command instead of maintaining a tracked document
list. Each system README defines its durable boundary and related systems.

Plans and work orders are in [`docs/plans`](../plans). Architecture decisions
are in [`docs/decisions`](../decisions). Public user documentation is in
[`docs/public`](../public).
