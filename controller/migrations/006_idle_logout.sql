ALTER TABLE user_policies ADD COLUMN idle_logout_minutes INTEGER NOT NULL DEFAULT 30 CHECK(idle_logout_minutes BETWEEN 1 AND 1440);
