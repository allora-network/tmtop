package display

import (
	"testing"
	"time"

	"main/pkg/metrics"
)

func TestValidatorHealthTableData_EmptyRows(t *testing.T) {
	d := NewValidatorHealthTableData(false)

	// Before any SetRows call the table has no cells.
	if d.GetRowCount() != 0 {
		t.Errorf("expected 0 rows, got %d", d.GetRowCount())
	}
	if d.GetColumnCount() != 0 {
		t.Errorf("expected 0 columns, got %d", d.GetColumnCount())
	}
}

func TestValidatorHealthTableData_HeaderAndSeparator(t *testing.T) {
	d := NewValidatorHealthTableData(false)
	d.SetRows([]metrics.ValidatorHealthRow{})

	// With zero validators we still get header + separator = 2 rows.
	if d.GetRowCount() != 2 {
		t.Errorf("expected 2 rows (header+sep), got %d", d.GetRowCount())
	}
	if d.GetColumnCount() != 13 {
		t.Errorf("expected 13 columns, got %d", d.GetColumnCount())
	}

	// Header row column names in order.
	wantHeaders := []string{"#", "Moniker", "VP%", "Sign%", "Missed", "Streak",
		"Prevote%", "Precommit%", "Prop%", "ArrPrevote", "ArrPrecommit", "ASN", "Status"}
	for i, want := range wantHeaders {
		cell := d.GetCell(0, i)
		if cell == nil {
			t.Fatalf("header cell nil at column %d", i)
		}
		if cell.Text != want {
			t.Errorf("header col %d: got %q, want %q", i, cell.Text, want)
		}
	}
}

func TestValidatorHealthTableData_DashWhenNoHistory(t *testing.T) {
	d := NewValidatorHealthTableData(false)
	row := metrics.ValidatorHealthRow{
		Address:        "valaddr1",
		Moniker:        "test-val",
		VotingPowerPct: 1.23,
		HasHistory:     false,
		HasLatency:     false,
		Online:         true,
	}
	d.SetRows([]metrics.ValidatorHealthRow{row})

	// row index 2 (0=header, 1=sep, 2=first data row)
	const dataRow = 2
	// Columns 3..8 (Sign%..Prop%) and 9..10 (arrival) should all be "—".
	dashCols := []int{3, 4, 5, 6, 7, 8, 9, 10}
	for _, col := range dashCols {
		cell := d.GetCell(dataRow, col)
		if cell == nil {
			t.Fatalf("cell nil at row=%d col=%d", dataRow, col)
		}
		if cell.Text != "—" {
			t.Errorf("row=%d col=%d: got %q, want dash", dataRow, col, cell.Text)
		}
	}
}

func TestValidatorHealthTableData_ValuesWhenHasHistory(t *testing.T) {
	d := NewValidatorHealthTableData(false)
	row := metrics.ValidatorHealthRow{
		Address:           "valaddr2",
		Moniker:           "healthy-val",
		VotingPowerPct:    2.50,
		HasHistory:        true,
		SigningRatePct:    99.1,
		BlocksMissed:      5,
		LongestMissStreak: 2,
		PrevoteRatePct:    98.0,
		PrecommitRatePct:  97.5,
		ProposerSharePct:  0.33,
		HasLatency:        true,
		AvgPrevoteArrival: 100 * time.Millisecond,
		AvgPrecommitArrival: 150 * time.Millisecond,
		Online:            true,
	}
	d.SetRows([]metrics.ValidatorHealthRow{row})

	const dataRow = 2
	// Columns 3..8 must NOT be "—".
	histCols := []int{3, 4, 5, 6, 7, 8}
	for _, col := range histCols {
		cell := d.GetCell(dataRow, col)
		if cell == nil {
			t.Fatalf("cell nil at row=%d col=%d", dataRow, col)
		}
		if cell.Text == "—" {
			t.Errorf("row=%d col=%d: unexpectedly got dash with HasHistory=true", dataRow, col)
		}
	}

	// Arrival columns must not be "—" when HasLatency=true.
	for _, col := range []int{9, 10} {
		cell := d.GetCell(dataRow, col)
		if cell == nil {
			t.Fatalf("cell nil at row=%d col=%d", dataRow, col)
		}
		if cell.Text == "—" {
			t.Errorf("row=%d col=%d: unexpectedly got dash with HasLatency=true", dataRow, col)
		}
	}
}

func TestValidatorHealthTableData_StatusEmoji(t *testing.T) {
	d := NewValidatorHealthTableData(false)
	rows := []metrics.ValidatorHealthRow{
		{Moniker: "online", Online: true, Equivocated: false},
		{Moniker: "offline", Online: false, Equivocated: false},
		{Moniker: "evil", Online: true, Equivocated: true},
	}
	d.SetRows(rows)

	tests := []struct {
		dataRow int
		want    string
	}{
		{2, "✅"},
		{3, "❌"},
		{4, "✅⚠"},
	}
	for _, tt := range tests {
		cell := d.GetCell(tt.dataRow, 12)
		if cell == nil {
			t.Fatalf("status cell nil at row=%d", tt.dataRow)
		}
		if cell.Text != tt.want {
			t.Errorf("row=%d status: got %q, want %q", tt.dataRow, cell.Text, tt.want)
		}
	}
}

func TestValidatorHealthTableData_StatusDisableEmojis(t *testing.T) {
	d := NewValidatorHealthTableData(true) // DisableEmojis
	rows := []metrics.ValidatorHealthRow{
		{Moniker: "online", Online: true, Equivocated: false},
		{Moniker: "offline", Online: false, Equivocated: false},
		{Moniker: "evil", Online: true, Equivocated: true},
	}
	d.SetRows(rows)

	tests := []struct {
		dataRow int
		want    string
	}{
		{2, "[on]"},
		{3, "[off]"},
		{4, "[on][!]"},
	}
	for _, tt := range tests {
		cell := d.GetCell(tt.dataRow, 12)
		if cell == nil {
			t.Fatalf("status cell nil at row=%d", tt.dataRow)
		}
		if cell.Text != tt.want {
			t.Errorf("row=%d status: got %q, want %q", tt.dataRow, cell.Text, tt.want)
		}
	}
}

func TestValidatorHealthTableData_RowCount(t *testing.T) {
	d := NewValidatorHealthTableData(false)
	rows := make([]metrics.ValidatorHealthRow, 5)
	for i := range rows {
		rows[i] = metrics.ValidatorHealthRow{Moniker: "val"}
	}
	d.SetRows(rows)

	// header(1) + separator(1) + 5 data rows = 7
	if d.GetRowCount() != 7 {
		t.Errorf("expected 7 rows, got %d", d.GetRowCount())
	}
	if d.GetColumnCount() != 13 {
		t.Errorf("expected 13 columns, got %d", d.GetColumnCount())
	}
}
