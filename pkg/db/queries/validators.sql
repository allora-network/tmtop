-- name: GetValidator :one
SELECT * FROM validators WHERE address = ? LIMIT 1;

-- name: GetValidators :many
SELECT * FROM validators ORDER BY voting_power DESC;

-- name: GetValidatorsByHeight :many
SELECT v.*, vs.voting_power, vs.voting_power_percent, vs.is_proposer
FROM validators v
JOIN validator_snapshots vs ON v.address = vs.validator_address
WHERE vs.height = ?
ORDER BY vs.voting_power DESC;

-- name: CreateValidator :one
INSERT INTO validators (address, public_key, voting_power, moniker)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: UpdateValidator :one
UPDATE validators 
SET public_key = ?, voting_power = ?, moniker = ?, updated_at = CURRENT_TIMESTAMP
WHERE address = ?
RETURNING *;

-- name: UpsertValidator :one
INSERT INTO validators (address, public_key, voting_power, moniker)
VALUES (?, ?, ?, ?)
ON CONFLICT(address) DO UPDATE SET
    public_key = excluded.public_key,
    voting_power = excluded.voting_power,
    moniker = excluded.moniker,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: DeleteValidator :exec
DELETE FROM validators WHERE address = ?;