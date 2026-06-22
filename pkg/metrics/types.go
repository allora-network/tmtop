package metrics

import "time"

// NetworkHealth holds network-wide consensus + decentralization metrics.
// Zero values render as "—"; each field is populated by exactly one workstream.
type NetworkHealth struct {
	// Liveness (WS-B)
	ChainHalted        bool
	SecondsSinceHeight float64
	CurrentMaxRound    int32
	OfflineCount       int
	OfflinePowerPct    float64

	// Consensus quality (WS-C)
	RoundsPerBlockAvg  float64
	RoundZeroCommitPct float64
	AvgBlockTime       time.Duration
	BlockTimeStdDev    time.Duration
	BlockIntervals     []time.Duration // recent, oldest→newest, for sparkline

	// Step timing (WS-H)
	AvgProposeTime   time.Duration
	AvgPrevoteTime   time.Duration
	AvgPrecommitTime time.Duration
	StepTimingSample int // number of observed blocks behind the averages

	// Mempool (WS-D)
	MempoolTxs   int64
	MempoolBytes int64
	MempoolKnown bool

	// Decentralization (WS-A)
	Nakamoto33 int // min validators whose combined power exceeds 1/3 (halt set)
	Nakamoto66 int // min validators whose combined power exceeds 2/3 (control set)
	Gini       float64
	Top10Pct   float64

	// ASN / hosting (WS-I)
	TopASNs []ASNShare

	// Security (WS-G)
	Equivocations []EquivocationEvent
}

// ASNShare is one hosting-provider's share of the active set.
type ASNShare struct {
	ASN         uint32
	Description string
	Validators  int
	PowerPct    float64
}

// EquivocationEvent records a validator signing two distinct block IDs for
// the same (height, round, vote type). Detection is limited to conflicts
// within a single consensus round — it does not cross rounds and does not
// compare PartSetHeader. A validator that votes for different blocks in
// different rounds (e.g. after a round timeout) will NOT be flagged.
type EquivocationEvent struct {
	ValidatorAddress string
	Moniker          string
	Height           int64
	Round            int32
	VoteType         string // "prevote" | "precommit"
	BlockIDA         string
	BlockIDB         string
	DetectedAt       time.Time
}

// ValidatorHealthRow is one row of the Validator Health table.
type ValidatorHealthRow struct {
	Address        string
	Moniker        string
	VotingPowerPct float64

	// Tier 1 DB-backed (HasHistory=false when DB disabled → render "—")
	HasHistory       bool
	SigningRatePct   float64
	BlocksMissed     int64
	PrevoteRatePct   float64
	PrecommitRatePct float64
	ProposerSharePct float64

	// Arrival latency, single-vantage (WS-F); zero ⇒ unavailable
	HasLatency          bool
	AvgPrevoteArrival   time.Duration
	AvgPrecommitArrival time.Duration
	MaxPrecommitArrival time.Duration

	// Live (WS-B)
	Online bool

	// ASN (WS-I)
	ASN    uint32
	ASNOrg string

	// Security (WS-G)
	Equivocated bool
}
