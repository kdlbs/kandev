# Bounded month-edge query correction

C11 is based on `1ce8d0bfdab088f899cf0ede7a5a911d097e0cee`. It changes one SQL builder and one scan regression. All 17 benchmark files remain unchanged.

The failed C10 receipt showed a point-index scan without date bounds. A large OR expression caused SQLite to visit 440,019 point entries in the one-year monthly case.

The correction drives indexed point probes from the existing UTC edge-day calendar. Calendar clipping and half-open bounds remain unchanged.

The negative control retained the new assertion and restored the old production query. The unchanged 2,217-fact fixture failed with 4,419 unbounded point visits. The restoration trap then restored the reviewed correction.

Focused checks passed on SQLite and PostgreSQL 18.6. They include nine monthly controls, all five deadline filters, full-row parity, MaxInt64, amount boundaries and same-database index replay. The monthly race suite passed on both databases. Lint reported no issues; vet confirmed exit zero.

The parent verified the source hashes, diff, protected files and actual logs before integration. These focused results support the query correction. Both fresh normal full-scale database gates remain required.

The normal commit hook later formatted the scan regression. The parent verified that Go formatting of the tested bytes produces the hook result exactly. No semantic source changed; the original receipts remain intact. See `normal-hook-format.json`.
