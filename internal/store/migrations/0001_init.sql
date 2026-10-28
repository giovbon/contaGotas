-- 0001_init — schema inicial (ver docs/data-model.md).
-- PRAGMAs de conexão ficam no DSN (internal/store/store.go).

CREATE TABLE documents (
  id             INTEGER PRIMARY KEY,
  title          TEXT    NOT NULL,
  author         TEXT,
  language       TEXT    NOT NULL DEFAULT 'pt-BR',
  format         TEXT    NOT NULL CHECK (format IN ('md','epub','pdf')),
  source_name    TEXT    NOT NULL,
  source_path    TEXT    NOT NULL,
  source_sha256  TEXT    NOT NULL UNIQUE,
  cover_path     TEXT,
  status         TEXT    NOT NULL DEFAULT 'processing'
                 CHECK (status IN ('processing','ready','paused','completed','failed','needs_ocr')),
  pacing_mode    TEXT    NOT NULL DEFAULT 'minutes_per_day'
                 CHECK (pacing_mode IN ('minutes_per_day','finish_by_days')),
  pacing_minutes INTEGER NOT NULL DEFAULT 10,
  pacing_days    INTEGER,
  wpm            INTEGER NOT NULL DEFAULT 200,
  difficulty     REAL    NOT NULL DEFAULT 1.0,
  priority       INTEGER NOT NULL DEFAULT 100,
  total_words    INTEGER NOT NULL DEFAULT 0,
  total_seconds  INTEGER NOT NULL DEFAULT 0,
  error          TEXT,
  created_at     TEXT    NOT NULL,
  updated_at     TEXT    NOT NULL
);
CREATE INDEX idx_documents_status ON documents(status, priority, id);

CREATE TABLE blocks (
  id          INTEGER PRIMARY KEY,
  document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  ord         INTEGER NOT NULL,
  kind        TEXT    NOT NULL CHECK (kind IN
              ('h1','h2','h3','h4','h5','h6','p','li','code','table','quote','figure','footnote','hr')),
  level       INTEGER NOT NULL DEFAULT 0,
  heading_id  INTEGER REFERENCES blocks(id) ON DELETE SET NULL,
  text        TEXT    NOT NULL,
  html        TEXT    NOT NULL DEFAULT '',
  words       INTEGER NOT NULL DEFAULT 0,
  seconds     INTEGER NOT NULL DEFAULT 0,
  hash        TEXT    NOT NULL,
  UNIQUE (document_id, ord)
);
CREATE INDEX idx_blocks_doc_ord ON blocks(document_id, ord);
CREATE INDEX idx_blocks_heading ON blocks(heading_id, ord);

CREATE TABLE plans (
  id             INTEGER PRIMARY KEY,
  document_id    INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  version        INTEGER NOT NULL,
  status         TEXT    NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active','superseded','stale')),
  pacing_mode    TEXT    NOT NULL,
  target_seconds INTEGER NOT NULL,
  horizon_days   INTEGER,
  params_json    TEXT    NOT NULL,
  created_at     TEXT    NOT NULL,
  UNIQUE (document_id, version)
);
CREATE UNIQUE INDEX ux_plans_active ON plans(document_id) WHERE status = 'active';

CREATE TABLE chunks (
  id                 INTEGER PRIMARY KEY,
  plan_id            INTEGER NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
  document_id        INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  seq                INTEGER NOT NULL,
  first_block_id     INTEGER NOT NULL REFERENCES blocks(id),
  last_block_id      INTEGER NOT NULL REFERENCES blocks(id),
  first_hash         TEXT    NOT NULL,
  last_hash          TEXT    NOT NULL,
  section_path       TEXT    NOT NULL,
  starts_mid_section INTEGER NOT NULL DEFAULT 0,
  ends_mid_section   INTEGER NOT NULL DEFAULT 0,
  oversized          INTEGER NOT NULL DEFAULT 0,
  words              INTEGER NOT NULL,
  seconds            INTEGER NOT NULL,
  target_seconds     INTEGER NOT NULL,
  status             TEXT    NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','sent','skipped')),
  sent_at            TEXT,
  done_at            TEXT,
  UNIQUE (plan_id, seq)
);
CREATE INDEX idx_chunks_cursor ON chunks(plan_id, status, seq);

CREATE TABLE delivery_runs (
  id            INTEGER PRIMARY KEY,
  run_date      TEXT    NOT NULL UNIQUE,
  scheduled_for TEXT    NOT NULL,
  started_at    TEXT    NOT NULL,
  finished_at   TEXT,
  status        TEXT    NOT NULL DEFAULT 'running'
                CHECK (status IN ('running','sent','partial','failed')),
  late          INTEGER NOT NULL DEFAULT 0,
  error         TEXT
);

CREATE TABLE delivery_items (
  id          INTEGER PRIMARY KEY,
  run_id      INTEGER NOT NULL REFERENCES delivery_runs(id) ON DELETE CASCADE,
  document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  chunk_id    INTEGER NOT NULL REFERENCES chunks(id),
  status      TEXT    NOT NULL DEFAULT 'pending'
              CHECK (status IN ('pending','sent','failed','skipped')),
  attempts    INTEGER NOT NULL DEFAULT 0,
  bytes       INTEGER,
  error       TEXT,
  sent_at     TEXT,
  UNIQUE (run_id, document_id)
);
CREATE INDEX idx_items_doc ON delivery_items(document_id, id);

CREATE TABLE settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE events (
  id          INTEGER PRIMARY KEY,
  at          TEXT NOT NULL,
  level       TEXT NOT NULL CHECK (level IN ('debug','info','warn','error')),
  kind        TEXT NOT NULL,
  document_id INTEGER,
  message     TEXT NOT NULL
);
CREATE INDEX idx_events_at ON events(at);
