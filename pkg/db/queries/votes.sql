-- name: GetVote :one
SELECT * FROM votes 
WHERE height = ? AND round_number = ? AND validator_address = ? AND vote_type = ?
LIMIT 1;

-- name: GetVotesForRound :many
SELECT * FROM votes 
WHERE height = ? AND round_number = ?
ORDER BY vote_type, validator_address;

-- name: GetVotesForHeight :many
SELECT * FROM votes WHERE height = ? ORDER BY round_number, vote_type, validator_address;

-- name: GetVotesForValidator :many
SELECT * FROM votes 
WHERE validator_address = ? AND height >= ?
ORDER BY height DESC, round_number DESC;

-- name: GetVotesByType :many
SELECT * FROM votes 
WHERE height = ? AND round_number = ? AND vote_type = ?
ORDER BY validator_address;

-- name: CreateVote :one
INSERT INTO votes (height, round_number, validator_address, vote_type, block_hash, signature, timestamp)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpsertVote :one
INSERT INTO votes (height, round_number, validator_address, vote_type, block_hash, signature, timestamp)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(height, round_number, validator_address, vote_type) DO UPDATE SET
    block_hash = excluded.block_hash,
    signature = excluded.signature,
    timestamp = excluded.timestamp
RETURNING *;

-- name: GetVotingPowerForRound :one
SELECT 
    SUM(CASE WHEN v.vote_type = 1 AND v.block_hash IS NOT NULL THEN vs.voting_power ELSE 0 END) as prevote_power,
    SUM(CASE WHEN v.vote_type = 2 AND v.block_hash IS NOT NULL THEN vs.voting_power ELSE 0 END) as precommit_power,
    SUM(vs.voting_power) as total_power
FROM votes v
JOIN validator_snapshots vs ON v.validator_address = vs.validator_address AND v.height = vs.height
WHERE v.height = ? AND v.round_number = ?;

-- name: DeleteVotesOlderThan :exec
DELETE FROM votes WHERE height < ?;