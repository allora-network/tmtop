-- Drop the UNIQUE constraint on validators.operator_address.
--
-- Rationale: hex_address is the canonical CometBFT identity; the
-- operator_address is metadata that can legitimately be missing
-- (validator info hasn't synced yet) or change (key rotation, or
-- the same DB file is shared between runs against different
-- chains). With both columns UNIQUE, an upsert that conflicts on
-- operator_address but not hex_address fails outright, which then
-- cascades into FOREIGN KEY violations on the rounds table when
-- it references the validator's hex_address.
--
-- SQLite has no ALTER TABLE DROP CONSTRAINT, so we recreate the
-- table without the constraint and copy the data over. Foreign
-- keys must be disabled during the rebuild because DROP TABLE on
-- a table referenced by other tables triggers ON DELETE NO ACTION
-- and fails (SQLITE_CONSTRAINT_FOREIGNKEY 787). PRAGMA foreign_keys
-- is a no-op inside a transaction, so it is set outside BEGIN/COMMIT.
-- See https://www.sqlite.org/lang_altertable.html#otheralter

PRAGMA foreign_keys=OFF;

-- Idempotent recovery: a prior failed run of this migration may
-- have left validators_new behind because each statement in a
-- multi-statement migration autocommits.
DROP TABLE IF EXISTS validators_new;

BEGIN TRANSACTION;

CREATE TABLE validators_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operator_address TEXT NOT NULL,
    hex_address TEXT NOT NULL UNIQUE,
    public_key TEXT NOT NULL,
    voting_power INTEGER NOT NULL,
    moniker TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO validators_new (id, operator_address, hex_address, public_key, voting_power, moniker, created_at, updated_at)
SELECT id, operator_address, hex_address, public_key, voting_power, moniker, created_at, updated_at FROM validators;

DROP TABLE validators;
ALTER TABLE validators_new RENAME TO validators;

CREATE INDEX idx_validators_hex_address ON validators(hex_address);
CREATE INDEX idx_validators_operator_address ON validators(operator_address);

COMMIT;

PRAGMA foreign_keys=ON;
