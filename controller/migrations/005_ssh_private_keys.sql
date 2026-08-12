CREATE TABLE credentials_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    username TEXT,
    encrypted_secret BLOB,
    secret_type TEXT NOT NULL CHECK(secret_type IN ('password','username_only','ssh_private_key')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

INSERT INTO credentials_new SELECT * FROM credentials;
DROP TABLE credentials;
ALTER TABLE credentials_new RENAME TO credentials;
