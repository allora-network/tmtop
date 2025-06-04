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

type AllRoundsTableData struct {
	tview.TableContentReadOnly

	RoundData     *types.RoundDataMap
	TMValidators  types.TMValidators
	DisableEmojis bool
	Transpose     bool
	CurrentHeight int64

	MaxHistorySize int

	cells [][]*tview.TableCell
	mutex *utils.NoopLocker
}

func NewAllRoundsTableData(disableEmojis bool, transpose bool) *AllRoundsTableData {
	return &AllRoundsTableData{
		DisableEmojis:  disableEmojis,
		Transpose:      transpose,
		MaxHistorySize: 10,
		cells:          [][]*tview.TableCell{},
		mutex:          &utils.NoopLocker{},
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

// SetTMValidators sets the unified validator collection (preferred)
func (d *AllRoundsTableData) SetTMValidators(validators types.TMValidators, height int64) {
	d.mutex.Lock()

	if d.CurrentHeight == 0 && height > 0 {
		d.CurrentHeight = height
	}
	d.TMValidators = validators

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

// Create cells for the table
func (d *AllRoundsTableData) createCells() [][]*tview.TableCell {
	cells := [][]*tview.TableCell{}

	// Check if we have validators to display
	if len(d.TMValidators) == 0 {
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

	// Create validator rows using TMValidators
	for i, validator := range d.TMValidators {
		row := []*tview.TableCell{}

		// enumerated validator name
		name := validator.GetDisplayName()
		row = append(row, tview.NewTableCell(fmt.Sprintf("%d. %s", i+1, name)))
		row = append(row, tview.NewTableCell(fmt.Sprintf("(%.2f%%)", validator.VotingPowerPercent)))

		for _, roundData := range d.RoundData.ReverseIter() {
			valVotes := roundData.Votes[validator.GetDisplayAddress()]

			// Use new VoteState functions with CometBFT types
			prevote := types.VoteStateFromVotesMap(valVotes, cmtproto.PrevoteType)
			precommit := types.VoteStateFromVotesMap(valVotes, cmtproto.PrecommitType)

			cell := tview.NewTableCell(" " + precommit.Serialize(d.DisableEmojis) + prevote.Serialize(d.DisableEmojis) + " ")
			if roundData.Proposers.Has(validator.GetDisplayAddress()) {
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
