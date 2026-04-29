-- name: GetVote :one
SELECT * FROM votes 
WHERE height = ? AND round_number = ? AND validator_hex_address = ? AND vote_type = ?
LIMIT 1;

-- name: GetVotesForRound :many
SELECT * FROM votes 
WHERE height = ? AND round_number = ?
ORDER BY vote_type, validator_hex_address;

-- name: GetVotesForHeight :many
SELECT * FROM votes WHERE height = ? ORDER BY round_number, vote_type, validator_hex_address;

-- name: GetVotesForValidator :many
SELECT * FROM votes 
WHERE validator_hex_address = ? AND height >= ?
ORDER BY height DESC, round_number DESC;

-- name: GetVotesByType :many
SELECT * FROM votes 
WHERE height = ? AND round_number = ? AND vote_type = ?
ORDER BY validator_hex_address;

-- name: CreateVote :one
INSERT INTO votes (height, round_number, validator_hex_address, vote_type, block_hash, signature, timestamp)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpsertVote :one
INSERT INTO votes (height, round_number, validator_hex_address, vote_type, block_hash, signature, timestamp)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(height, round_number, validator_hex_address, vote_type) DO UPDATE SET
    block_hash = excluded.block_hash,
    signature = excluded.signature,
    timestamp = excluded.timestamp
RETURNING *;

-- name: GetVotingPowerForRound :one
-- total_power deliberately uses a separate subquery rather than SUM(vs.voting_power)
-- over the join: a validator with both a prevote and precommit row would appear
-- twice in the join result, which would double-count its voting power in any
-- aggregate that doesn't filter by vote_type.
SELECT
    SUM(CASE WHEN v.vote_type = 1 AND v.block_hash IS NOT NULL THEN vs.voting_power ELSE 0 END) as prevote_power,
    SUM(CASE WHEN v.vote_type = 2 AND v.block_hash IS NOT NULL THEN vs.voting_power ELSE 0 END) as precommit_power,
    (SELECT COALESCE(SUM(vs2.voting_power), 0) FROM validator_snapshots vs2 WHERE vs2.height = ?) as total_power
FROM votes v
JOIN validator_snapshots vs ON v.validator_hex_address = vs.validator_hex_address AND v.height = vs.height
WHERE v.height = ? AND v.round_number = ?;

-- name: DeleteVotesOlderThan :exec
DELETE FROM votes WHERE height < ?;