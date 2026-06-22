-- name: GetMaxStoredHeight :one
SELECT COALESCE(MAX(height), 0) AS height FROM heights;

-- name: GetValidatorRankingByHeight :many
-- Signing efficiency + participation for all validators over a height window
-- [min_height, max_height]. Counts distinct heights to avoid the prevote+precommit
-- double-count (see analytics.sql notes).
WITH validator_metrics AS (
    SELECT
        vals.hex_address,
        vals.moniker,
        COUNT(DISTINCT h.height) AS total_blocks,
        COUNT(DISTINCT CASE WHEN v.id IS NOT NULL THEN h.height END) AS blocks_signed,
        COALESCE(ROUND(
            100.0 * COUNT(DISTINCT CASE WHEN v.id IS NOT NULL THEN h.height END)
                  / NULLIF(COUNT(DISTINCT h.height), 0), 2), 0.0) AS signing_efficiency,
        COUNT(CASE WHEN v.vote_type = 1 THEN 1 END) AS prevotes_cast,
        COUNT(CASE WHEN v.vote_type = 2 THEN 1 END) AS precommits_cast,
        COALESCE(MAX(vs.voting_power), 0) AS voting_power
    FROM validators vals
    CROSS JOIN heights h
    LEFT JOIN votes v ON vals.hex_address = v.validator_hex_address AND h.height = v.height
    LEFT JOIN validator_snapshots vs ON vals.hex_address = vs.validator_hex_address AND h.height = vs.height
    WHERE h.height >= ? AND h.height <= ?
    GROUP BY vals.hex_address, vals.moniker
)
SELECT
    hex_address, moniker, total_blocks, blocks_signed,
    total_blocks - blocks_signed AS blocks_missed,
    signing_efficiency, prevotes_cast, precommits_cast, voting_power
FROM validator_metrics
ORDER BY signing_efficiency DESC;

-- name: GetProposerPerformanceByHeight :many
-- Per-validator proposer share over a height window: how often each validator
-- proposed relative to all proposing opportunities.
SELECT
    r.proposer_address AS hex_address,
    COUNT(*) AS blocks_proposed
FROM rounds r
WHERE r.height >= ? AND r.height <= ? AND r.proposer_address IS NOT NULL
GROUP BY r.proposer_address;

-- name: GetVoteArrivalByHeight :many
-- Per-validator, per-vote-type arrival latency relative to round start, using
-- locally-observed timestamps (single-vantage). Returns avg/max in seconds.
SELECT
    v.validator_hex_address AS hex_address,
    v.vote_type AS vote_type,
    COUNT(*) AS samples,
    COALESCE(AVG((julianday(v.timestamp) - julianday(r.start_time)) * 86400.0), 0.0) AS avg_secs,
    COALESCE(MAX((julianday(v.timestamp) - julianday(r.start_time)) * 86400.0), 0.0) AS max_secs
FROM votes v
JOIN rounds r ON v.height = r.height AND v.round_number = r.round_number
WHERE v.height >= ? AND v.height <= ?
  AND v.timestamp IS NOT NULL AND r.start_time IS NOT NULL
GROUP BY v.validator_hex_address, v.vote_type;
