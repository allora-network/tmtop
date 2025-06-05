-- name: GetValidatorSnapshot :one
SELECT * FROM validator_snapshots 
WHERE height = ? AND validator_address = ? 
LIMIT 1;

-- name: GetValidatorSnapshotsForHeight :many
SELECT * FROM validator_snapshots 
WHERE height = ?
ORDER BY voting_power DESC;

-- name: GetValidatorSnapshotsForValidator :many
SELECT * FROM validator_snapshots 
WHERE validator_address = ? AND height >= ?
ORDER BY height DESC;

-- name: CreateValidatorSnapshot :one
INSERT INTO validator_snapshots (height, validator_address, voting_power, voting_power_percent, is_proposer)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: UpsertValidatorSnapshot :one
INSERT INTO validator_snapshots (height, validator_address, voting_power, voting_power_percent, is_proposer)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(height, validator_address) DO UPDATE SET
    voting_power = excluded.voting_power,
    voting_power_percent = excluded.voting_power_percent,
    is_proposer = excluded.is_proposer
RETURNING *;

-- name: BatchCreateValidatorSnapshots :exec
INSERT INTO validator_snapshots (height, validator_address, voting_power, voting_power_percent, is_proposer)
VALUES (?, ?, ?, ?, ?);

-- name: DeleteValidatorSnapshotsOlderThan :exec
DELETE FROM validator_snapshots WHERE height < ?;