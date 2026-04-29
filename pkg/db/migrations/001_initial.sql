-- Initial schema for tmtop database

-- Validators table to store validator information
CREATE TABLE validators (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operator_address TEXT NOT NULL UNIQUE,
    hex_address TEXT NOT NULL UNIQUE,
    public_key TEXT NOT NULL,
    voting_power INTEGER NOT NULL,
    moniker TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast validator lookups
CREATE INDEX idx_validators_hex_address ON validators(hex_address);
CREATE INDEX idx_validators_operator_address ON validators(operator_address);

-- Heights table to store block height information
CREATE TABLE heights (
    height INTEGER PRIMARY KEY,
    block_hash TEXT,
    block_time DATETIME,
    proposer_address TEXT,
    total_validators INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (proposer_address) REFERENCES validators(hex_address)
);

-- Rounds table to store consensus round information
CREATE TABLE rounds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    height INTEGER NOT NULL,
    round_number INTEGER NOT NULL,
    step INTEGER,
    start_time DATETIME,
    proposer_address TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(height, round_number),
    FOREIGN KEY (height) REFERENCES heights(height),
    FOREIGN KEY (proposer_address) REFERENCES validators(hex_address)
);

-- Index for efficient round queries
CREATE INDEX idx_rounds_height_round ON rounds(height, round_number);
CREATE INDEX idx_rounds_height_desc ON rounds(height DESC);

-- Votes table to store individual validator votes
CREATE TABLE votes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    height INTEGER NOT NULL,
    round_number INTEGER NOT NULL,
    validator_hex_address TEXT NOT NULL,
    vote_type INTEGER NOT NULL, -- 1 = prevote, 2 = precommit
    block_hash TEXT, -- NULL for nil votes
    signature TEXT,
    timestamp DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(height, round_number, validator_hex_address, vote_type),
    FOREIGN KEY (height) REFERENCES heights(height),
    FOREIGN KEY (validator_hex_address) REFERENCES validators(hex_address)
);

-- Indexes for efficient vote queries
CREATE INDEX idx_votes_height_round ON votes(height, round_number);
CREATE INDEX idx_votes_validator ON votes(validator_hex_address);
CREATE INDEX idx_votes_height_round_type ON votes(height, round_number, vote_type);

-- Consensus events table for tracking important consensus milestones
CREATE TABLE consensus_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    height INTEGER NOT NULL,
    round_number INTEGER NOT NULL,
    event_type TEXT NOT NULL, -- 'new_round', 'proposal', 'prevote_majority', 'precommit_majority', 'block_commit'
    event_data TEXT, -- JSON data for additional context
    timestamp DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (height) REFERENCES heights(height)
);

-- Index for consensus event queries
CREATE INDEX idx_consensus_events_height_round ON consensus_events(height, round_number);
CREATE INDEX idx_consensus_events_type ON consensus_events(event_type);
CREATE INDEX idx_consensus_events_timestamp ON consensus_events(timestamp);

-- Validator snapshots table to track voting power changes over time
CREATE TABLE validator_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    height INTEGER NOT NULL,
    validator_hex_address TEXT NOT NULL,
    voting_power INTEGER NOT NULL,
    voting_power_percent REAL,
    is_proposer BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(height, validator_hex_address),
    FOREIGN KEY (height) REFERENCES heights(height),
    FOREIGN KEY (validator_hex_address) REFERENCES validators(hex_address)
);

-- Index for efficient validator snapshot queries
CREATE INDEX idx_validator_snapshots_height ON validator_snapshots(height);
CREATE INDEX idx_validator_snapshots_validator ON validator_snapshots(validator_hex_address);