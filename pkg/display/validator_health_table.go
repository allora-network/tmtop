package display

import (
	"fmt"
	"strconv"

	"main/pkg/metrics"
	"main/pkg/utils"

	"github.com/rivo/tview"
)

// ValidatorHealthTableData implements tview.TableContent for the Validator
// Health mode. It mirrors the net_info_table.go pattern: a
// tview.TableContentReadOnly embed, a cells grid, and a NoopLocker for
// thread-safety (the tview draw loop is single-threaded; the locker exists
// to satisfy the same structural contract as sibling tables).
type ValidatorHealthTableData struct {
	tview.TableContentReadOnly

	rows          []metrics.ValidatorHealthRow
	disableEmojis bool

	cells [][]*tview.TableCell
	mutex *utils.NoopLocker
}

func NewValidatorHealthTableData(disableEmojis bool) *ValidatorHealthTableData {
	return &ValidatorHealthTableData{
		cells:         [][]*tview.TableCell{},
		mutex:         &utils.NoopLocker{},
		disableEmojis: disableEmojis,
	}
}

func (d *ValidatorHealthTableData) GetCell(row, column int) *tview.TableCell {
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

func (d *ValidatorHealthTableData) GetRowCount() int {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	return len(d.cells)
}

func (d *ValidatorHealthTableData) GetColumnCount() int {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	if len(d.cells) == 0 {
		return 0
	}
	return len(d.cells[0])
}

// SetRows replaces the validator rows and re-renders the cell grid.
func (d *ValidatorHealthTableData) SetRows(rows []metrics.ValidatorHealthRow) {
	d.rows = rows
	d.redrawData()
}

func (d *ValidatorHealthTableData) redrawData() {
	cells := d.makeCells()

	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.cells = cells
}

// makeCells builds the full [][]*tview.TableCell from the current rows slice.
// Layout: header row, separator row, one row per validator (2 + len(rows) total).
func (d *ValidatorHealthTableData) makeCells() [][]*tview.TableCell {
	const numCols = 13

	// header + separator + one row per validator
	cells := make([][]*tview.TableCell, len(d.rows)+2)

	// ── Header ──────────────────────────────────────────────────────────────
	cells[0] = []*tview.TableCell{
		tview.NewTableCell("#"),
		tview.NewTableCell("Moniker"),
		tview.NewTableCell("VP%"),
		tview.NewTableCell("Sign%"),
		tview.NewTableCell("Missed"),
		tview.NewTableCell("Streak"),
		tview.NewTableCell("Prevote%"),
		tview.NewTableCell("Precommit%"),
		tview.NewTableCell("Prop%"),
		tview.NewTableCell("ArrPrevote"),
		tview.NewTableCell("ArrPrecommit"),
		tview.NewTableCell("ASN"),
		tview.NewTableCell("Status"),
	}

	// ── Separator ───────────────────────────────────────────────────────────
	cells[1] = []*tview.TableCell{
		tview.NewTableCell("="),
		tview.NewTableCell("======="),
		tview.NewTableCell("===="),
		tview.NewTableCell("====="),
		tview.NewTableCell("======"),
		tview.NewTableCell("======"),
		tview.NewTableCell("========"),
		tview.NewTableCell("=========="),
		tview.NewTableCell("====="),
		tview.NewTableCell("=========="),
		tview.NewTableCell("============"),
		tview.NewTableCell("==="),
		tview.NewTableCell("======"),
	}

	// ── Data rows ───────────────────────────────────────────────────────────
	for i, row := range d.rows {
		r := make([]*tview.TableCell, numCols)

		// # (1-based index)
		r[0] = tview.NewTableCell(strconv.Itoa(i + 1)).SetAlign(tview.AlignRight)

		// Moniker
		r[1] = tview.NewTableCell(utils.RightPadAndTrim(row.Moniker, 20))

		// VP%
		r[2] = tview.NewTableCell(fmt.Sprintf("%.2f", row.VotingPowerPct)).SetAlign(tview.AlignRight)

		// History-backed columns: Sign%, Missed, Streak, Prevote%, Precommit%, Prop%
		dash := "—"
		if row.HasHistory {
			r[3] = tview.NewTableCell(fmt.Sprintf("%.2f", row.SigningRatePct)).SetAlign(tview.AlignRight)
			r[4] = tview.NewTableCell(strconv.FormatInt(row.BlocksMissed, 10)).SetAlign(tview.AlignRight)
			r[5] = tview.NewTableCell(strconv.FormatInt(row.LongestMissStreak, 10)).SetAlign(tview.AlignRight)
			r[6] = tview.NewTableCell(fmt.Sprintf("%.2f", row.PrevoteRatePct)).SetAlign(tview.AlignRight)
			r[7] = tview.NewTableCell(fmt.Sprintf("%.2f", row.PrecommitRatePct)).SetAlign(tview.AlignRight)
			r[8] = tview.NewTableCell(fmt.Sprintf("%.2f", row.ProposerSharePct)).SetAlign(tview.AlignRight)
		} else {
			r[3] = tview.NewTableCell(dash).SetAlign(tview.AlignRight)
			r[4] = tview.NewTableCell(dash).SetAlign(tview.AlignRight)
			r[5] = tview.NewTableCell(dash).SetAlign(tview.AlignRight)
			r[6] = tview.NewTableCell(dash).SetAlign(tview.AlignRight)
			r[7] = tview.NewTableCell(dash).SetAlign(tview.AlignRight)
			r[8] = tview.NewTableCell(dash).SetAlign(tview.AlignRight)
		}

		// Latency-backed columns: ArrPrevote, ArrPrecommit
		if row.HasLatency {
			r[9] = tview.NewTableCell(row.AvgPrevoteArrival.String()).SetAlign(tview.AlignRight)
			r[10] = tview.NewTableCell(row.AvgPrecommitArrival.String()).SetAlign(tview.AlignRight)
		} else {
			r[9] = tview.NewTableCell(dash).SetAlign(tview.AlignRight)
			r[10] = tview.NewTableCell(dash).SetAlign(tview.AlignRight)
		}

		// ASN
		if row.ASN != 0 {
			asnText := fmt.Sprintf("AS%d", row.ASN)
			if row.ASNOrg != "" {
				asnText = utils.RightPadAndTrim(row.ASNOrg, 12)
			}
			r[11] = tview.NewTableCell(asnText)
		} else {
			r[11] = tview.NewTableCell(dash)
		}

		// Status: Online indicator + optional equivocation warning
		r[12] = tview.NewTableCell(d.serializeStatus(row.Online, row.Equivocated))

		cells[i+2] = r
	}

	return cells
}

// serializeStatus renders the Online/Offline and Equivocated flags, honouring
// DisableEmojis the same way last_round_table.go honours it for vote states.
func (d *ValidatorHealthTableData) serializeStatus(online, equivocated bool) string {
	var status string
	if online {
		if d.disableEmojis {
			status = "[on]"
		} else {
			status = "✅"
		}
	} else {
		if d.disableEmojis {
			status = "[off]"
		} else {
			status = "❌"
		}
	}

	if equivocated {
		if d.disableEmojis {
			status += "[!]"
		} else {
			status += "⚠"
		}
	}

	return status
}
