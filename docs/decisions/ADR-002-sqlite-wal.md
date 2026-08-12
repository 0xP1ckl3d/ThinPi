# ADR-002: SQLite WAL and single-connection policy

Status: accepted, 2026-08-11.

Enable SQLite WAL, foreign keys, and a five-second busy timeout. Restrict the
initial Go connection pool to one connection. WAL separates readers/writers and
improves recovery characteristics for the small controller, while a single
application connection avoids surprising lock contention. Revisit with measured
controller load before changing databases or pool size.
