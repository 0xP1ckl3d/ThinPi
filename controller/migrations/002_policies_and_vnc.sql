CREATE TABLE connections_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    protocol TEXT NOT NULL CHECK(protocol IN ('rdp','moonlight','vnc','mock')),
    host TEXT NOT NULL,
    port INTEGER NOT NULL CHECK(port BETWEEN 1 AND 65535),
    enabled INTEGER NOT NULL DEFAULT 1,
    icon TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    protocol_config_json TEXT NOT NULL DEFAULT '{}',
    credential_id INTEGER REFERENCES credentials(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

INSERT INTO connections_new SELECT * FROM connections;
DROP TABLE connections;
ALTER TABLE connections_new RENAME TO connections;

CREATE TABLE user_policies (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    timezone TEXT NOT NULL DEFAULT 'Australia/Sydney',
    allowed_days_mask INTEGER NOT NULL DEFAULT 127 CHECK(allowed_days_mask BETWEEN 0 AND 127),
    access_start_minute INTEGER NOT NULL DEFAULT 0 CHECK(access_start_minute BETWEEN 0 AND 1439),
    access_end_minute INTEGER NOT NULL DEFAULT 1440 CHECK(access_end_minute BETWEEN 1 AND 1440),
    daily_limit_minutes INTEGER NOT NULL DEFAULT 0 CHECK(daily_limit_minutes BETWEEN 0 AND 1440),
    max_session_minutes INTEGER NOT NULL DEFAULT 0 CHECK(max_session_minutes BETWEEN 0 AND 720),
    updated_at TEXT NOT NULL
);

ALTER TABLE launch_tickets ADD COLUMN max_session_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_connection_permissions ADD COLUMN credential_id INTEGER REFERENCES credentials(id) ON DELETE SET NULL;
ALTER TABLE group_connection_permissions ADD COLUMN credential_id INTEGER REFERENCES credentials(id) ON DELETE SET NULL;

CREATE TABLE session_usage (
    ticket_id INTEGER PRIMARY KEY REFERENCES launch_tickets(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    connection_id INTEGER NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    started_at TEXT NOT NULL,
    ended_at TEXT,
    duration_seconds INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_session_usage_user_started ON session_usage(user_id, started_at);
