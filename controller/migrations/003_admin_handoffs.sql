CREATE TABLE admin_handoffs (
    token_hash TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    redeemed_at TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_admin_handoffs_expiry ON admin_handoffs(expires_at);
