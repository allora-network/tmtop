-- name: GetConsensusEvent :one
SELECT * FROM consensus_events WHERE id = ? LIMIT 1;

-- name: GetConsensusEventsForRound :many
SELECT * FROM consensus_events 
WHERE height = ? AND round_number = ?
ORDER BY timestamp;

-- name: GetConsensusEventsForHeight :many
SELECT * FROM consensus_events WHERE height = ? ORDER BY round_number, timestamp;

-- name: GetRecentConsensusEvents :many
SELECT * FROM consensus_events ORDER BY timestamp DESC LIMIT ?;

-- name: GetConsensusEventsByType :many
SELECT * FROM consensus_events 
WHERE height >= ? AND event_type = ?
ORDER BY height DESC, round_number DESC, timestamp DESC;

-- name: CreateConsensusEvent :one
INSERT INTO consensus_events (height, round_number, event_type, event_data, timestamp)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteConsensusEventsOlderThan :exec
DELETE FROM consensus_events WHERE height < ?;