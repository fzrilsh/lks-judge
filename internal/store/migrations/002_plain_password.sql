-- Adds plain_password to participants for DBs created before 001 carried it.
-- Guarded by a pragma check in migrate(): only runs when the column is absent,
-- since SQLite ALTER TABLE ADD COLUMN has no IF NOT EXISTS.
ALTER TABLE participants ADD COLUMN plain_password TEXT;
