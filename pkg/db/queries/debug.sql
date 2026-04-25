-- COUNT helpers — one per table because sqlc can't parameterize
-- table identifiers.

-- name: CountValidators :one
SELECT COUNT(*) FROM validators;

-- name: CountHeights :one
SELECT COUNT(*) FROM heights;

-- name: CountRounds :one
SELECT COUNT(*) FROM rounds;

-- name: CountVotes :one
SELECT COUNT(*) FROM votes;

-- name: CountConsensusEvents :one
SELECT COUNT(*) FROM consensus_events;

-- name: CountValidatorSnapshots :one
SELECT COUNT(*) FROM validator_snapshots;

-- name: SampleValidators :many
SELECT operator_address, moniker, voting_power FROM validators LIMIT 5;

-- name: SampleHeights :many
SELECT height, block_time, proposer_address FROM heights ORDER BY height DESC LIMIT 5;

-- name: SampleVotes :many
SELECT height, round_number, validator_hex_address, vote_type, timestamp FROM votes LIMIT 5;

-- name: ValidatorExistsByOperator :one
SELECT COUNT(*) FROM validators WHERE operator_address = ?;

-- name: ListAllValidators :many
SELECT operator_address, moniker FROM validators ORDER BY operator_address;

-- name: CountHeightsInTimeWindow :one
SELECT COUNT(*) FROM heights WHERE block_time >= ? AND block_time <= ?;

-- name: CountVotesForValidatorInTimeWindow :one
SELECT COUNT(*)
FROM votes v
JOIN heights h ON v.height = h.height
JOIN validators val ON v.validator_hex_address = val.hex_address
WHERE val.operator_address = ? AND h.block_time >= ? AND h.block_time <= ?;

-- name: SampleVotesForValidator :many
SELECT v.height, v.round_number, v.vote_type, v.timestamp, h.block_time
FROM votes v
JOIN heights h ON v.height = h.height
JOIN validators val ON v.validator_hex_address = val.hex_address
WHERE val.operator_address = ?
ORDER BY v.height DESC
LIMIT ?;

-- name: SearchValidatorsByOperatorOrMoniker :many
SELECT operator_address, moniker, voting_power
FROM validators
WHERE operator_address LIKE ? OR LOWER(moniker) LIKE LOWER(?)
ORDER BY operator_address;
