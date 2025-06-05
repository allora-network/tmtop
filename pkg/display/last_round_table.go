package display

import (
	"fmt"
	"main/pkg/types"
	"main/pkg/utils"
	"strconv"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type LastRoundTableData struct {
	tview.TableContentReadOnly

	TMValidators   types.TMValidators
	RoundData      *types.RoundDataMap
	CurrentHeight  int64
	CurrentRound   int32
	ConsensusError error
	ColumnsCount   int
	DisableEmojis  bool
	Transpose      bool

	cells [][]*tview.TableCell
	mutex *utils.NoopLocker
}

func NewLastRoundTableData(columnsCount int, disableEmojis bool, transpose bool) *LastRoundTableData {
	return &LastRoundTableData{
		ColumnsCount:  columnsCount,
		TMValidators:  make(types.TMValidators, 0),
		RoundData:     types.NewRoundDataMap(),
		CurrentHeight: 0,
		CurrentRound:  0,
		DisableEmojis: disableEmojis,
		Transpose:     transpose,

		cells: [][]*tview.TableCell{},
		mutex: &utils.NoopLocker{},
	}
}

func (d *LastRoundTableData) SetColumnsCount(count int) {
	d.mutex.Lock()
	d.ColumnsCount = count
	d.mutex.Unlock()

	d.redrawData()
}

func (d *LastRoundTableData) SetTranspose(transpose bool) {
	d.mutex.Lock()
	d.Transpose = transpose
	d.mutex.Unlock()

	d.redrawData()
}

func (d *LastRoundTableData) GetCell(row, column int) *tview.TableCell {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	if len(d.cells) <= row {
		return nil
	}

	if len(d.cells[row]) <= column {
		return nil
	}

	return d.cells[row][column]
}

func (d *LastRoundTableData) GetRowCount() int {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	return len(d.cells)
}

func (d *LastRoundTableData) GetColumnCount() int {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	if len(d.cells) == 0 {
		return 0
	}

	return len(d.cells[0])
}

// SetTMValidators sets the unified validator collection (preferred)
func (d *LastRoundTableData) SetTMValidators(validators types.TMValidators, consensusError error) {
	d.mutex.Lock()
	d.TMValidators = validators
	d.ConsensusError = consensusError
	d.mutex.Unlock()

	d.redrawData()
}

// SetRoundData sets the round data map for vote tracking
func (d *LastRoundTableData) SetRoundData(roundData *types.RoundDataMap) {
	d.mutex.Lock()
	d.RoundData = roundData
	d.mutex.Unlock()

	d.redrawData()
}

// SetCurrentRound sets the current height and round for display
func (d *LastRoundTableData) SetCurrentRound(height int64, round int32) {
	d.mutex.Lock()
	d.CurrentHeight = height
	d.CurrentRound = round
	d.mutex.Unlock()

	d.redrawData()
}

func (d *LastRoundTableData) redrawData() {
	cells := d.makeCells()

	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.cells = cells
}

func (d *LastRoundTableData) makeCells() [][]*tview.TableCell {
	if d.ConsensusError != nil {
		return [][]*tview.TableCell{
			{tview.NewTableCell(fmt.Sprintf(" Error fetching consensus: %s", d.ConsensusError))},
		}
	} else if d.ColumnsCount == 0 {
		return [][]*tview.TableCell{}
	}

	// Use TMValidators
	validatorCount := len(d.TMValidators)

	rowsCount := validatorCount/d.ColumnsCount + 1
	if validatorCount%d.ColumnsCount == 0 {
		rowsCount = validatorCount / d.ColumnsCount
	}

	cells := make([][]*tview.TableCell, rowsCount)

	for row := 0; row < rowsCount; row++ {
		cells[row] = make([]*tview.TableCell, d.ColumnsCount)

		for column := 0; column < d.ColumnsCount; column++ {
			index := row*d.ColumnsCount + column

			if d.Transpose {
				rows := d.GetRowCount()
				index = column*rows + row
			}

			text := ""
			isProposer := false

			// Use TMValidators with RoundDataMap for vote state
			if index < len(d.TMValidators) {
				validator := d.TMValidators[index]

				// Check if validator is proposer for current round
				if d.RoundData != nil {
					proposers := d.RoundData.GetProposers(d.CurrentHeight, d.CurrentRound)
					isProposer = proposers != nil && proposers.Has(validator.GetDisplayAddress())
				}

				// Generate display text using RoundDataMap vote data
				text = d.generateValidatorDisplayText(validator)
			}

			cell := tview.NewTableCell(text)

			if isProposer {
				cell.SetBackgroundColor(tcell.ColorForestGreen)
			}

			cells[row][column] = cell
		}
	}
	return cells
}

// generateValidatorDisplayText creates validator display text using RoundDataMap data
func (d *LastRoundTableData) generateValidatorDisplayText(validator types.TMValidator) string {
	name := validator.GetDisplayName()
	if validator.HasAssignedKey() {
		emoji := "🔑"
		if d.DisableEmojis {
			emoji = "[k]"
		}
		name = emoji + " " + name
	}

	// Default vote states (no vote)
	prevoteStr := "❌"
	precommitStr := "❌"
	if d.DisableEmojis {
		prevoteStr = "[ ]"
		precommitStr = "[ ]"
	}

	// Query RoundDataMap for current vote states
	if d.RoundData != nil && d.CurrentHeight > 0 {
		prevoteState := d.RoundData.GetVote(d.CurrentHeight, d.CurrentRound, validator.GetDisplayAddress(), cmtproto.PrevoteType)
		precommitState := d.RoundData.GetVote(d.CurrentHeight, d.CurrentRound, validator.GetDisplayAddress(), cmtproto.PrecommitType)

		prevoteStr = prevoteState.Serialize(d.DisableEmojis)
		precommitStr = precommitState.Serialize(d.DisableEmojis)
	}

	// Format voting power percentage
	votingPowerStr := "0.00"
	if validator.VotingPowerPercent != nil {
		votingPowerStr = validator.VotingPowerPercent.Text('f', 2)
	}

	return fmt.Sprintf(
		" %s %s %s %s%% %s ",
		prevoteStr,
		precommitStr,
		utils.RightPadAndTrim(strconv.Itoa(validator.Index+1), 3),
		utils.RightPadAndTrim(votingPowerStr, 6),
		utils.LeftPadAndTrim(name, 25),
	)
}
