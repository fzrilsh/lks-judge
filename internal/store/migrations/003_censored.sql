-- Adds censored to competitions: when 1 the public leaderboard hides per-module
-- scores, total score and rank, and shows rows in random order. Guarded by a
-- pragma check in migrate() since SQLite ALTER TABLE ADD COLUMN has no IF NOT EXISTS.
ALTER TABLE competitions ADD COLUMN censored INTEGER NOT NULL DEFAULT 0;
