package display

import (
	"fmt"
	"strings"
	"time"

	"main/pkg/types"
	"main/pkg/utils"
)

// SerializeConsensus renders the top-left consensus summary block.
func SerializeConsensus(s *types.State, timezone *time.Location) string {
	if err := s.GetConsensusStateError(); err != nil {
		return fmt.Sprintf(" consensus state error: %s", err)
	}

	validators := s.GetTMValidators()
	if len(validators) == 0 {
		return ""
	}

	height, round, step, startTime := s.GetConsensusHeight()

	var sb strings.Builder

	fmt.Fprintf(&sb, " height=%d round=%d step=%d\n", height, round, step)
	fmt.Fprintf(&sb,
		" block time: %s (%s)\n",
		utils.ZeroOrPositiveDuration(utils.SerializeDuration(time.Since(startTime))),
		utils.SerializeTime(startTime.In(timezone)),
	)
	fmt.Fprintf(&sb,
		" prevote consensus (total/agreeing): %.2f / %.2f\n",
		s.GetTotalVotingPowerPrevotedPercent(true),
		s.GetTotalVotingPowerPrevotedPercent(false),
	)
	fmt.Fprintf(&sb,
		" precommit consensus (total/agreeing): %.2f / %.2f\n",
		s.GetTotalVotingPowerPrecommittedPercent(true),
		s.GetTotalVotingPowerPrecommittedPercent(false),
	)

	fmt.Fprintf(&sb, " last updated at: %s\n", utils.SerializeTime(time.Now().In(timezone)))

	return sb.String()
}

// SerializeChainInfo renders the top-middle chain info block.
func SerializeChainInfo(s *types.State, timezone *time.Location) string {
	var sb strings.Builder

	rpc := s.CurrentRPC()
	fmt.Fprintf(&sb, " rpc: %v\n", rpc.URL)
	fmt.Fprintf(&sb, " (%v)\n\n", rpc.Moniker)

	chainInfo := s.GetChainInfo()
	blockTime := s.GetBlockTime()
	if chainInfoErr := s.GetChainInfoError(); chainInfoErr != nil {
		fmt.Fprintf(&sb, " chain info fetch error: %s\n", chainInfoErr.Error())
	} else if chainInfo != nil {
		fmt.Fprintf(&sb, " chain name: %s\n", chainInfo.NodeInfo.Network)
		fmt.Fprintf(&sb, " tendermint version: v%s\n", chainInfo.NodeInfo.Version)

		if blockTime != 0 {
			fmt.Fprintf(&sb, " avg block time: %s\n", utils.SerializeDuration(blockTime))
		}
	}

	upgrade := s.GetUpgrade()
	if upgradePlanErr := s.GetUpgradePlanError(); upgradePlanErr != nil {
		fmt.Fprintf(&sb, " upgrade plan fetch error: %s\n", upgradePlanErr)
	} else if upgrade == nil {
		sb.WriteString(" no chain upgrade scheduled\n")
	} else {
		height, _, _, _ := s.GetConsensusHeight()
		sb.WriteString(serializeUpgradeInfo(upgrade, height, blockTime, timezone))
	}

	return sb.String()
}

func serializeUpgradeInfo(upgrade *types.Upgrade, height int64, blockTime time.Duration, timezone *time.Location) string {
	var sb strings.Builder

	if upgrade.Height+1 == height {
		sb.WriteString(" upgrade in progress...\n")
		return sb.String()
	}

	if upgrade.Height+1 < height {
		fmt.Fprintf(&sb,
			" chain upgrade %s applied at block %d\n",
			upgrade.Name,
			upgrade.Height,
		)
		fmt.Fprintf(&sb,
			" blocks since upgrade: %d\n",
			height-upgrade.Height,
		)

		if blockTime == 0 {
			return sb.String()
		}

		upgradeTime := utils.CalculateTimeTillBlock(height, upgrade.Height, blockTime)
		fmt.Fprintf(&sb,
			" time since upgrade: %s\n",
			utils.SerializeDuration(time.Since(upgradeTime)),
		)
		fmt.Fprintf(&sb, " upgrade approximate time: %s\n", utils.SerializeTime(upgradeTime.In(timezone)))
		return sb.String()
	}

	fmt.Fprintf(&sb,
		" chain upgrade %s scheduled at block %d\n",
		upgrade.Name,
		upgrade.Height,
	)
	fmt.Fprintf(&sb,
		" blocks till upgrade: %d\n",
		upgrade.Height-height,
	)

	if blockTime == 0 {
		return sb.String()
	}

	upgradeTime := utils.CalculateTimeTillBlock(height, upgrade.Height, blockTime)

	fmt.Fprintf(&sb,
		" time till upgrade: %s\n",
		utils.SerializeDuration(time.Until(upgradeTime)),
	)
	fmt.Fprintf(&sb, " upgrade estimated time: %s\n", utils.SerializeTime(upgradeTime.In(timezone)))

	return sb.String()
}

// SerializePrevotesProgressbar renders the prevote progress bar for the
// current height/round.
func SerializePrevotesProgressbar(s *types.State, width int, height int) string {
	if len(s.GetTMValidators()) == 0 {
		return ""
	}

	prevotePercent := s.GetTotalVotingPowerPrevotedPercent(true)
	prevotePercentFloat, _ := prevotePercent.Float64()
	return progressBar(width, height, "Prevotes: ", int(prevotePercentFloat))
}

// SerializePrecommitsProgressbar renders the precommit progress bar.
func SerializePrecommitsProgressbar(s *types.State, width int, height int) string {
	if len(s.GetTMValidators()) == 0 {
		return ""
	}

	precommitPercent := s.GetTotalVotingPowerPrecommittedPercent(true)
	precommitPercentFloat, _ := precommitPercent.Float64()
	return progressBar(width, height, "Precommits: ", int(precommitPercentFloat))
}

func progressBar(width int, height int, prefix string, progress int) string {
	return ProgressBar{
		Width:    width,
		Height:   height,
		Progress: progress,
		Prefix:   prefix,
	}.Serialize()
}
