CREATE TABLE sst_db_viability_records (
    correlation_id text PRIMARY KEY,
    event_type text NOT NULL,
    created_at timestamptz NOT NULL
);
