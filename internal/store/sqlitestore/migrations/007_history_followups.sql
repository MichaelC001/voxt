-- VoxtMini database, migration 7 (11-14): the follow-up questions asked
-- about an entry in its details and their answers (store.Entry.FollowUps,
-- a JSON array; '' for none). Only SetFollowUps writes it.
ALTER TABLE history ADD COLUMN followups TEXT NOT NULL DEFAULT '';
