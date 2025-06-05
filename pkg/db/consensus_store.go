package db

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"main/pkg/db/sqlc"
	"main/pkg/types"

	cptypes "github.com/cometbft/cometbft/proto/tendermint/types"
	ctypes "github.com/cometbft/cometbft/types"
	"github.com/rs/zerolog"
)

// ConsensusStore handles persistence of consensus data
type ConsensusStore struct {
	db     *DB
	logger zerolog.Logger
}

// NewConsensusStore creates a new consensus store
func NewConsensusStore(db *DB, logger zerolog.Logger) *ConsensusStore {
	return &ConsensusStore{
		db:     db,
		logger: logger.With().Str("component", "consensus_store").Logger(),
	}
}

// StoreRoundData persists round data from RoundDataMap
func (cs *ConsensusStore) StoreRoundData(ctx context.Context, height int64, round int32, roundData *types.RoundData, validators types.TMValidators) error {
	return cs.db.WithTx(ctx, func(q *sqlc.Queries) error {
		// Store the round
		roundParams := sqlc.UpsertRoundParams{
			Height:      height,
			RoundNumber: int64(round),
			Step:        sql.NullInt64{}, // We can add step tracking later
			StartTime: sql.NullTime{
				Time:  time.Now(),
				Valid: true,
			},
			ProposerAddress: sql.NullString{},
		}

		// Set proposer if we have one
		if roundData.Proposers != nil && len(roundData.Proposers) > 0 {
			// Get first proposer from the set
			for proposer := range roundData.Proposers {
				roundParams.ProposerAddress = sql.NullString{
					String: proposer,
					Valid:  true,
				}
				break // Just get the first one
			}
		}

		if _, err := q.UpsertRound(ctx, roundParams); err != nil {
			return fmt.Errorf("failed to store round: %w", err)
		}

		// Store votes for this round
		for validatorAddr, voteMap := range roundData.Votes {
			for voteType, blockID := range voteMap {
				voteParams := sqlc.UpsertVoteParams{
					Height:           height,
					RoundNumber:      int64(round),
					ValidatorAddress: validatorAddr,
					VoteType:         int64(voteType),
					Signature:        sql.NullString{}, // We can add signature tracking later
					Timestamp: sql.NullTime{
						Time:  time.Now(),
						Valid: true,
					},
				}

				// Set block hash if not nil vote
				if !blockID.IsZero() {
					voteParams.BlockHash = sql.NullString{
						String: blockID.Hash.String(),
						Valid:  true,
					}
				}

				if _, err := q.UpsertVote(ctx, voteParams); err != nil {
					cs.logger.Error().Err(err).
						Str("validator", validatorAddr).
						Int64("height", height).
						Int32("round", round).
						Int64("vote_type", int64(voteType)).
						Msg("Failed to store vote")
					// Continue with other votes instead of failing completely
				}
			}
		}

		return nil
	})
}

// StoreValidators persists validator information and creates snapshots for a height
func (cs *ConsensusStore) StoreValidators(ctx context.Context, height int64, validators types.TMValidators) error {
	return cs.db.WithTx(ctx, func(q *sqlc.Queries) error {
		// Store height record first
		heightParams := sqlc.UpsertHeightParams{
			Height:           height,
			BlockHash:        sql.NullString{}, // Can be added later
			BlockTime:        sql.NullTime{},   // Can be added later
			ProposerAddress:  sql.NullString{}, // Can be added later
			TotalValidators:  sql.NullInt64{Int64: int64(len(validators)), Valid: true},
		}

		if _, err := q.UpsertHeight(ctx, heightParams); err != nil {
			return fmt.Errorf("failed to store height: %w", err)
		}

		// Store validators and their snapshots
		for _, validator := range validators {
			// Upsert validator
			validatorParams := sqlc.UpsertValidatorParams{
				Address:     validator.GetDisplayAddress(),
				PublicKey:   hex.EncodeToString(validator.PubKey.Bytes()),
				VotingPower: validator.VotingPower,
				Moniker:     sql.NullString{},
			}

			// Add moniker if available
			if validator.ChainValidator != nil && validator.ChainValidator.Moniker != "" {
				validatorParams.Moniker = sql.NullString{
					String: validator.ChainValidator.Moniker,
					Valid:  true,
				}
			}

			if _, err := q.UpsertValidator(ctx, validatorParams); err != nil {
				cs.logger.Error().Err(err).
					Str("validator", validator.GetDisplayAddress()).
					Msg("Failed to store validator")
				continue
			}

			// Create validator snapshot for this height
			votingPowerPercent := 0.0
			if validator.VotingPowerPercent != nil {
				if pct, _ := validator.VotingPowerPercent.Float64(); pct >= 0 {
					votingPowerPercent = pct
				}
			}

			snapshotParams := sqlc.UpsertValidatorSnapshotParams{
				Height:              height,
				ValidatorAddress:    validator.GetDisplayAddress(),
				VotingPower:         validator.VotingPower,
				VotingPowerPercent:  sql.NullFloat64{Float64: votingPowerPercent, Valid: true},
				IsProposer:          sql.NullBool{}, // Will be set when we know the proposer
			}

			if _, err := q.UpsertValidatorSnapshot(ctx, snapshotParams); err != nil {
				cs.logger.Error().Err(err).
					Str("validator", validator.GetDisplayAddress()).
					Int64("height", height).
					Msg("Failed to store validator snapshot")
			}
		}

		return nil
	})
}

// StoreConsensusEvent records a consensus milestone event
func (cs *ConsensusStore) StoreConsensusEvent(ctx context.Context, height int64, round int32, eventType string, eventData interface{}) error {
	var eventDataJSON sql.NullString

	if eventData != nil {
		data, err := json.Marshal(eventData)
		if err != nil {
			cs.logger.Warn().Err(err).Msg("Failed to marshal event data")
		} else {
			eventDataJSON = sql.NullString{String: string(data), Valid: true}
		}
	}

	params := sqlc.CreateConsensusEventParams{
		Height:      height,
		RoundNumber: int64(round),
		EventType:   eventType,
		EventData:   eventDataJSON,
		Timestamp:   time.Now(),
	}

	_, err := cs.db.queries.CreateConsensusEvent(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to store consensus event: %w", err)
	}

	return nil
}

// StoreCometBFTEvents processes and stores CometBFT events
func (cs *ConsensusStore) StoreCometBFTEvents(ctx context.Context, events []ctypes.TMEventData, validators types.TMValidators) error {
	for _, event := range events {
		switch x := event.(type) {
		case ctypes.EventDataNewRound:
			// Store new round event
			if err := cs.StoreConsensusEvent(ctx, x.Height, x.Round, "new_round", map[string]interface{}{
				"proposer": x.Proposer.Address.String(),
			}); err != nil {
				cs.logger.Error().Err(err).Int64("height", x.Height).Int32("round", x.Round).Msg("Failed to store new round event")
			}

			// Update proposer in validator snapshots
			if err := cs.updateProposerInSnapshots(ctx, x.Height, x.Proposer.Address.String()); err != nil {
				cs.logger.Error().Err(err).Int64("height", x.Height).Str("proposer", x.Proposer.Address.String()).Msg("Failed to update proposer")
			}

		case ctypes.EventDataVote:
			// Individual votes are handled in StoreRoundData, but we can track vote events here
			voteType := "prevote"
			if x.Vote.Type == cptypes.PrecommitType {
				voteType = "precommit"
			}

			if err := cs.StoreConsensusEvent(ctx, x.Vote.Height, x.Vote.Round, voteType, map[string]interface{}{
				"validator": x.Vote.ValidatorAddress.String(),
				"block_id":  x.Vote.BlockID.Hash.String(),
			}); err != nil {
				cs.logger.Debug().Err(err).Msg("Failed to store vote event")
			}
		}
	}

	return nil
}

// updateProposerInSnapshots updates the is_proposer flag for validator snapshots
func (cs *ConsensusStore) updateProposerInSnapshots(ctx context.Context, height int64, proposerAddr string) error {
	// First, clear any existing proposer flags for this height
	_, err := cs.db.db.ExecContext(ctx, 
		"UPDATE validator_snapshots SET is_proposer = FALSE WHERE height = ?", 
		height)
	if err != nil {
		return fmt.Errorf("failed to clear proposer flags: %w", err)
	}

	// Then set the current proposer
	_, err = cs.db.db.ExecContext(ctx, 
		"UPDATE validator_snapshots SET is_proposer = TRUE WHERE height = ? AND validator_address = ?", 
		height, proposerAddr)
	if err != nil {
		return fmt.Errorf("failed to set proposer flag: %w", err)
	}

	return nil
}

// LoadRoundDataMap loads historical round data from the database
func (cs *ConsensusStore) LoadRoundDataMap(ctx context.Context, fromHeight, toHeight int64) (*types.RoundDataMap, error) {
	roundDataMap := types.NewRoundDataMap()

	// Get rounds in the specified range
	rounds, err := cs.db.queries.GetRoundsInRange(ctx, sqlc.GetRoundsInRangeParams{
		Height:   fromHeight,
		Height_2: toHeight,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get rounds: %w", err)
	}

	for _, round := range rounds {
		height := round.Height
		roundNum := int32(round.RoundNumber)

		// Get votes for this round
		votes, err := cs.db.queries.GetVotesForRound(ctx, sqlc.GetVotesForRoundParams{
			Height:      height,
			RoundNumber: round.RoundNumber,
		})
		if err != nil {
			cs.logger.Error().Err(err).Int64("height", height).Int64("round", round.RoundNumber).Msg("Failed to get votes for round")
			continue
		}

		// Add proposer to round data
		if round.ProposerAddress.Valid {
			roundDataMap.AddProposer(height, roundNum, round.ProposerAddress.String)
		}

		// Add votes to round data
		for _, vote := range votes {
			var blockID ctypes.BlockID
			if vote.BlockHash.Valid {
				// Parse block hash - simplified for now
				blockID = ctypes.BlockID{
					Hash: []byte(vote.BlockHash.String),
				}
			}

			roundDataMap.AddVote(height, roundNum, vote.ValidatorAddress, 
				cptypes.SignedMsgType(vote.VoteType), blockID)
		}
	}

	return roundDataMap, nil
}

// GetRecentRounds returns recent round data for display
func (cs *ConsensusStore) GetRecentRounds(ctx context.Context, limit int32) ([]sqlc.Round, error) {
	return cs.db.queries.GetRecentRounds(ctx, int64(limit))
}

// GetValidatorsForHeight returns validator snapshots for a specific height
func (cs *ConsensusStore) GetValidatorsForHeight(ctx context.Context, height int64) ([]sqlc.GetValidatorsByHeightRow, error) {
	return cs.db.queries.GetValidatorsByHeight(ctx, height)
}