CREATE TABLE connections_ssh (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    protocol TEXT NOT NULL CHECK(protocol IN ('rdp','moonlight','vnc','ssh','mock')),
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

INSERT INTO connections_ssh SELECT * FROM connections;
DROP TABLE connections;
ALTER TABLE connections_ssh RENAME TO connections;

CREATE TABLE maintenance_tickets (
    token_hash TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    redeemed_at TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_maintenance_tickets_expiry ON maintenance_tickets(expires_at);
