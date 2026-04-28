package display

import (
	"fmt"
	"strconv"

	"main/pkg/types"
	"main/pkg/utils"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type AllRoundsTableData struct {
	tview.TableContentReadOnly

	RoundData     *types.RoundDataMap
	Validators    []types.TMValidator
	DisableEmojis bool
	Transpose     bool
	CurrentHeight int64
	CurrentRound  int32

	cells [][]*tview.TableCell
	mutex *utils.NoopLocker
}

func NewAllRoundsTableData(disableEmojis bool, transpose bool) *AllRoundsTableData {
	return &AllRoundsTableData{
		DisableEmojis: disableEmojis,
		Transpose:     transpose,
		cells:         [][]*tview.TableCell{},
		mutex:         &utils.NoopLocker{},
	}
}

func (d *AllRoundsTableData) GetCell(row, column int) *tview.TableCell {
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

func (d *AllRoundsTableData) GetRowCount() int {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	return len(d.cells)
}

func (d *AllRoundsTableData) GetColumnCount() int {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	if len(d.cells) == 0 {
		return 0
	}

	return len(d.cells[0])
}

func (d *AllRoundsTableData) SetTMValidators(validators types.TMValidators, height int64) {
	d.mutex.Lock()

	if d.CurrentHeight == 0 && height > 0 {
		d.CurrentHeight = height
	}
	d.Validators = validators

	d.mutex.Unlock()
}

// SetCurrentRound records the round number we should treat as "now"
// for proposer-row highlighting.
func (d *AllRoundsTableData) SetCurrentRound(height int64, round int32) {
	d.mutex.Lock()
	d.CurrentHeight = height
	d.CurrentRound = round
	d.mutex.Unlock()
}

func (d *AllRoundsTableData) SetRoundData(roundData *types.RoundDataMap) {
	d.mutex.Lock()
	d.RoundData = roundData
	d.mutex.Unlock()
}

func (d *AllRoundsTableData) SetTranspose(transpose bool) {
	d.mutex.Lock()
	d.Transpose = transpose
	d.mutex.Unlock()
}

func (d *AllRoundsTableData) Update() {
	cells := d.createCells()

	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.cells = cells
}

// Create cells for the table.
func (d *AllRoundsTableData) createCells() [][]*tview.TableCell {
	cells := [][]*tview.TableCell{}

	// Check if we have validators to display
	if len(d.Validators) == 0 {
		return cells
	}

	// Create header row with bold text
	headerRow := []*tview.TableCell{
		tview.NewTableCell("Validator").
			SetSelectable(false).
			SetStyle(tcell.StyleDefault.Bold(true)),

		tview.NewTableCell("").
			SetSelectable(false),
	}

	for hr := range d.RoundData.ReverseIter() {
		// Format height to show only last 4 digits -> this can be adjusted by preference
		heightStr := strconv.Itoa(int(hr.Height))
		if len(heightStr) > 4 {
			heightStr = heightStr[len(heightStr)-4:]
		}

		headerCell := tview.NewTableCell(fmt.Sprintf("%s.%d", heightStr, hr.Round)).
			SetSelectable(false).
			SetStyle(tcell.StyleDefault.Bold(true))
		headerRow = append(headerRow, headerCell)
	}
	cells = append(cells, headerRow)

	// Identify the current proposer (if any) so we can highlight the
	// whole row across the screen, not just the per-round proposer cell.
	currentProposers := d.RoundData.GetProposers(d.CurrentHeight, d.CurrentRound)

	// Create validator rows using TMValidators
	for i, validator := range d.Validators {
		row := []*tview.TableCell{}

		isCurrentProposer := currentProposers != nil && currentProposers.Has(validator.GetDisplayAddress())

		// enumerated validator name
		name := validator.GetDisplayName()
		nameCell := tview.NewTableCell(fmt.Sprintf("%d. %s", i+1, name))
		vpCell := tview.NewTableCell(fmt.Sprintf("(%.2f%%)", validator.VotingPowerPercent))
		if isCurrentProposer {
			nameCell.SetBackgroundColor(tcell.ColorForestGreen)
			vpCell.SetBackgroundColor(tcell.ColorForestGreen)
		}
		row = append(row, nameCell, vpCell)

		for _, roundData := range d.RoundData.ReverseIter() {
			valVotes := roundData.Votes[validator.GetDisplayAddress()]

			// Use new VoteState functions with CometBFT types
			prevote := types.VoteStateFromVotesMap(valVotes, cmtproto.PrevoteType)
			precommit := types.VoteStateFromVotesMap(valVotes, cmtproto.PrecommitType)

			cell := tview.NewTableCell(" " + precommit.Serialize(d.DisableEmojis) + prevote.Serialize(d.DisableEmojis) + " ")
			// Two highlight conditions, both green:
			//   1. This validator is the proposer for the current
			//      (in-progress) height/round — entire row gets the bar.
			//   2. This validator was the proposer for THIS specific
			//      historical round — just that round's cell.
			if isCurrentProposer || roundData.Proposers.Has(validator.GetDisplayAddress()) {
				cell.SetBackgroundColor(tcell.ColorForestGreen)
			}
			row = append(row, cell)
		}

		cells = append(cells, row)
	}

	return cells
}

func (d *AllRoundsTableData) HandleKey(event *tcell.EventKey) bool {
	return false
}
