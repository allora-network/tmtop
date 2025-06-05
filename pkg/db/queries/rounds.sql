-- name: GetRound :one
SELECT * FROM rounds WHERE height = ? AND round_number = ? LIMIT 1;

-- name: GetRoundsForHeight :many
SELECT * FROM rounds WHERE height = ? ORDER BY round_number;

-- name: GetRecentRounds :many
SELECT * FROM rounds ORDER BY height DESC, round_number DESC LIMIT ?;

-- name: GetRoundsInRange :many
SELECT * FROM rounds 
WHERE height >= ? AND height <= ?
ORDER BY height DESC, round_number DESC;

-- name: CreateRound :one
INSERT INTO rounds (height, round_number, step, start_time, proposer_address)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: UpsertRound :one
INSERT INTO rounds (height, round_number, step, start_time, proposer_address)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(height, round_number) DO UPDATE SET
    step = excluded.step,
    start_time = excluded.start_time,
    proposer_address = excluded.proposer_address
RETURNING *;

-- name: UpdateRoundStep :one
UPDATE rounds SET step = ? WHERE height = ? AND round_number = ? RETURNING *;

-- name: DeleteRoundsOlderThan :exec
DELETE FROM rounds WHERE height < ?;