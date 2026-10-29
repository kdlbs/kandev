# Fixture occurrence collision

The first full SQLite command failed during baseline construction after 99.534 seconds.
No timed report completed. PostgreSQL did not start.
The failure was a unique canonical fact key constraint in the benchmark fixture.
The old fixture assigned only four occurrence times per UTC day.
Dense external observations therefore repeated a coverage identity despite distinct source record IDs.

The tagged seed now spreads observations across distinct whole-microsecond occurrence times.
It divides the interval before multiplication to avoid overflow for five-year spans.
This correction preserves every raw observation and each independently selected canonical fact.
It does not change a production query, index, deadline, oracle, or measurement counter.

The regression checks 1,464 external observations in one year against one work group.
It requires distinct canonical keys and strictly increasing occurrence times.
The current small run passed SourceIdentities, Fixture, and ScanVisits.
Scoped lint returned zero issues after this correction.

The attempt1 log, exit status, and source manifest preserve the failed receipt.
The main SQLite log and source manifest identify the retry.
