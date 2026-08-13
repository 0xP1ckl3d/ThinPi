CREATE TABLE kiosk_settings (
    id INTEGER PRIMARY KEY CHECK(id = 1),
    screen_sleep_minutes INTEGER NOT NULL DEFAULT 15 CHECK(screen_sleep_minutes BETWEEN 0 AND 1440),
    show_user_list INTEGER NOT NULL DEFAULT 1 CHECK(show_user_list IN (0, 1)),
    terminal_theme TEXT NOT NULL DEFAULT 'dark' CHECK(terminal_theme IN ('dark', 'light')),
    client_theme TEXT NOT NULL DEFAULT 'ocean' CHECK(client_theme IN ('ocean', 'graphite', 'forest', 'sunset', 'high-contrast')),
    updated_at TEXT NOT NULL
);

INSERT INTO kiosk_settings(id, screen_sleep_minutes, show_user_list, terminal_theme, client_theme, updated_at)
VALUES(1, 15, 1, 'dark', 'ocean', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

ALTER TABLE users ADD COLUMN profile_photo BLOB;
ALTER TABLE users ADD COLUMN profile_photo_mime TEXT CHECK(profile_photo_mime IN ('image/png', 'image/jpeg', 'image/webp'));
