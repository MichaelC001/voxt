-- VoxtMini database, migration 6 (11-14): the dictionary's categories,
-- a term's category, its app enhancement group and how often sessions
-- used it (store.Term), and what the dictionary changed in a session's
-- text (store.Entry.Corrections, a JSON array; '' for none).
--
-- category and group_id are ids ('' = the default category, every app);
-- a category or group that no longer exists counts as ''. last_used_at
-- is Unix ms, 0 for never.
ALTER TABLE dictionary_terms ADD COLUMN category TEXT NOT NULL DEFAULT '';
ALTER TABLE dictionary_terms ADD COLUMN group_id TEXT NOT NULL DEFAULT '';
ALTER TABLE dictionary_terms ADD COLUMN uses INTEGER NOT NULL DEFAULT 0;
ALTER TABLE dictionary_terms ADD COLUMN last_used_at INTEGER NOT NULL DEFAULT 0;

CREATE TABLE dictionary_categories (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL COLLATE NOCASE UNIQUE CHECK (name != ''),
  created_at INTEGER NOT NULL
);

ALTER TABLE history ADD COLUMN corrections TEXT NOT NULL DEFAULT '';
