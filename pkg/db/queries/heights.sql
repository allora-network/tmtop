-- name: GetHeight :one
SELECT * FROM heights WHERE height = ? LIMIT 1;

-- name: GetHeights :many
SELECT * FROM heights ORDER BY height DESC LIMIT ?;

-- name: GetLatestHeight :one
SELECT * FROM heights ORDER BY height DESC LIMIT 1;

-- name: CreateHeight :one
INSERT INTO heights (height, block_hash, block_time, proposer_address, total_validators)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: UpsertHeight :one
INSERT INTO heights (height, block_hash, block_time, proposer_address, total_validators)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(height) DO UPDATE SET
    block_hash = excluded.block_hash,
    block_time = excluded.block_time,
    proposer_address = excluded.proposer_address,
    total_validators = excluded.total_validators
RETURNING *;

-- name: DeleteHeightsOlderThan :exec
DELETE FROM heights WHERE height < ?;