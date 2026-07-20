-- The read hot-path (LatestObservationAny's ORDER BY timestamp DESC LIMIT 1,
-- HistoryPoints' WHERE timestamp BETWEEN ?) filters/sorts on timestamp alone.
-- idx_obs_serial_time (0001_init.sql) leads with serial_number, so neither
-- query can use it and both fall back to a full table scan + sort, serialized
-- against the single writer connection (SGE review I1). This index leads
-- with timestamp so both queries can use it directly.
CREATE INDEX IF NOT EXISTS idx_obs_time ON tempest_observations(timestamp);
