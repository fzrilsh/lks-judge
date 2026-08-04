-- migration: 001_initial
-- guard: run once via schema_migrations table

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- skip if already applied
INSERT OR IGNORE INTO schema_migrations (version) VALUES (1);

CREATE TABLE IF NOT EXISTS competitions (
    id                INTEGER PRIMARY KEY,
    name              TEXT NOT NULL,
    level             TEXT NOT NULL,
    allowed_ips       TEXT NOT NULL DEFAULT '[]',
    current_module_id INTEGER REFERENCES modules(id) ON DELETE SET NULL,
    start_date        DATE NOT NULL,
    end_date          DATE NOT NULL,
    status            TEXT NOT NULL DEFAULT 'waiting',
    remaining_seconds INTEGER,
    paused_at         DATETIME,
    start_time        TIME,
    end_time          TIME,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS modules (
    id             INTEGER PRIMARY KEY,
    competition_id INTEGER NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    "order"        INTEGER NOT NULL,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS participants (
    id             INTEGER PRIMARY KEY,
    competition_id INTEGER NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    school         TEXT NOT NULL,
    pc_number      INTEGER,
    password       TEXT NOT NULL,
    plain_password TEXT,
    ip_address     TEXT,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS files (
    id             TEXT PRIMARY KEY,
    competition_id INTEGER NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    path           TEXT NOT NULL,
    is_public      INTEGER NOT NULL DEFAULT 0,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS submissions (
    id             TEXT PRIMARY KEY,
    participant_id INTEGER NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    module_id      INTEGER NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    file_path      TEXT NOT NULL,
    submitted_at   DATETIME,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(participant_id, module_id)
);

CREATE TABLE IF NOT EXISTS scores (
    id             INTEGER PRIMARY KEY,
    participant_id INTEGER NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    module_id      INTEGER NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
    score          INTEGER,
    wsi_score      INTEGER,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(participant_id, module_id)
);

CREATE TABLE IF NOT EXISTS sessions (
    token      TEXT PRIMARY KEY,
    owner_id   INTEGER NOT NULL,
    expires_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS upload_sessions (
    id             TEXT PRIMARY KEY,
    uploader_id    INTEGER NOT NULL,
    uploader_role  TEXT NOT NULL,
    competition_id INTEGER NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
    module_id      INTEGER REFERENCES modules(id) ON DELETE CASCADE,
    filename       TEXT NOT NULL,
    total_chunks   INTEGER NOT NULL,
    total_size     INTEGER NOT NULL,
    upload_type    TEXT NOT NULL,
    expires_at     DATETIME NOT NULL,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- indexes: all hot lookup paths
CREATE INDEX IF NOT EXISTS idx_sessions_token     ON sessions(token);
CREATE INDEX IF NOT EXISTS idx_participants_pc    ON participants(competition_id, pc_number);
CREATE INDEX IF NOT EXISTS idx_participants_ip    ON participants(ip_address);
CREATE INDEX IF NOT EXISTS idx_scores_lookup      ON scores(participant_id, module_id);
CREATE INDEX IF NOT EXISTS idx_submissions_lookup ON submissions(participant_id, module_id);
CREATE INDEX IF NOT EXISTS idx_files_competition  ON files(competition_id, is_public);
