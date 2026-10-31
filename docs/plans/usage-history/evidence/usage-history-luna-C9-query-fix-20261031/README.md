# Focused query review

This packet records the query correction from the isolated C9 checkout.
Its source base is `5b79da2f41f9a6e2c9bd9f421313276981e95c00`.
The parent reviewed all eight Go files and approved integration into the core PR.

The focused suite passed on SQLite and PostgreSQL 18.6.
It covered the five previous timeout filters and nine monthly controls.
The monthly controls compare complete rows and totals with the reference path.
The race run covered the monthly controls on both databases.
Lint and vet passed.

The control for independent metrics modifies a published contribution and rebuilds summaries.
It exercises the report behavior for absent cost.
The current capture path represents that input as unknown cost.

The full SQLite and PostgreSQL gates remain pending.
Each gate requires one million native observations, 100,000 external observations, and ten samples.
The original fixture, oracle, and report deadline remain required.

The original worker receipt contained a typo for the elapsed time of the earlier C8 command.
The reviewed receipt records the correct 3,618.666 seconds (60m18.666s).
The parent review records that correction.

- [source.diff](source.diff) contains the changes to eight Go files.
- [focused-ready.json](focused-ready.json) records the focused results and source hashes.
- [parent-review.json](parent-review.json) records the review and remaining gates.
- [file-manifest.json](file-manifest.json) identifies the source of each copied artifact.
- [logs](logs) contains the actual command output.
- [SHA256SUMS](SHA256SUMS) contains the packet hashes.
