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
	if s.ConsensusStateError != nil {
		return fmt.Sprintf(" consensus state error: %s", s.ConsensusStateError)
	}

	if len(s.TMValidators) == 0 {
		return ""
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, " height=%d round=%d step=%d\n", s.Height, s.Round, s.Step)
	fmt.Fprintf(&sb,
		" block time: %s (%s)\n",
		utils.ZeroOrPositiveDuration(utils.SerializeDuration(time.Since(s.StartTime))),
		utils.SerializeTime(s.StartTime.In(timezone)),
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

	fmt.Fprintf(&sb, " rpc: %v\n", s.CurrentRPC().URL)
	fmt.Fprintf(&sb, " (%v)\n\n", s.CurrentRPC().Moniker)

	if s.ChainInfoError != nil {
		fmt.Fprintf(&sb, " chain info fetch error: %s\n", s.ChainInfoError.Error())
	} else if s.ChainInfo != nil {
		fmt.Fprintf(&sb, " chain name: %s\n", s.ChainInfo.NodeInfo.Network)
		fmt.Fprintf(&sb, " tendermint version: v%s\n", s.ChainInfo.NodeInfo.Version)

		if s.BlockTime != 0 {
			fmt.Fprintf(&sb, " avg block time: %s\n", utils.SerializeDuration(s.BlockTime))
		}
	}

	if s.UpgradePlanError != nil {
		fmt.Fprintf(&sb, " upgrade plan fetch error: %s\n", s.UpgradePlanError)
	} else if s.Upgrade == nil {
		sb.WriteString(" no chain upgrade scheduled\n")
	} else {
		sb.WriteString(serializeUpgradeInfo(s, timezone))
	}

	return sb.String()
}

func serializeUpgradeInfo(s *types.State, timezone *time.Location) string {
	var sb strings.Builder

	if s.Upgrade.Height+1 == s.Height {
		sb.WriteString(" upgrade in progress...\n")
		return sb.String()
	}

	if s.Upgrade.Height+1 < s.Height {
		fmt.Fprintf(&sb,
			" chain upgrade %s applied at block %d\n",
			s.Upgrade.Name,
			s.Upgrade.Height,
		)
		fmt.Fprintf(&sb,
			" blocks since upgrade: %d\n",
			s.Height-s.Upgrade.Height,
		)

		if s.BlockTime == 0 {
			return sb.String()
		}

		upgradeTime := utils.CalculateTimeTillBlock(s.Height, s.Upgrade.Height, s.BlockTime)
		fmt.Fprintf(&sb,
			" time since upgrade: %s\n",
			utils.SerializeDuration(time.Since(upgradeTime)),
		)
		fmt.Fprintf(&sb, " upgrade approximate time: %s\n", utils.SerializeTime(upgradeTime.In(timezone)))
		return sb.String()
	}

	fmt.Fprintf(&sb,
		" chain upgrade %s scheduled at block %d\n",
		s.Upgrade.Name,
		s.Upgrade.Height,
	)
	fmt.Fprintf(&sb,
		" blocks till upgrade: %d\n",
		s.Upgrade.Height-s.Height,
	)

	if s.BlockTime == 0 {
		return sb.String()
	}

	upgradeTime := utils.CalculateTimeTillBlock(s.Height, s.Upgrade.Height, s.BlockTime)

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
	if len(s.TMValidators) == 0 {
		return ""
	}

	prevotePercent := s.GetTotalVotingPowerPrevotedPercent(true)
	prevotePercentFloat, _ := prevotePercent.Float64()
	return progressBar(width, height, "Prevotes: ", int(prevotePercentFloat))
}

// SerializePrecommitsProgressbar renders the precommit progress bar.
func SerializePrecommitsProgressbar(s *types.State, width int, height int) string {
	if len(s.TMValidators) == 0 {
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
