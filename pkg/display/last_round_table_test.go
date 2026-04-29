package display

import (
	"strings"
	"testing"

	"main/pkg/types"

	"github.com/cometbft/cometbft/crypto/ed25519"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	ctypes "github.com/cometbft/cometbft/types"
	"github.com/gdamore/tcell/v2"
)

func TestLastRoundTable_HighlightsCurrentProposer(t *testing.T) {
	pks := []ed25519.PubKey{
		ed25519.GenPrivKey().PubKey().(ed25519.PubKey),
		ed25519.GenPrivKey().PubKey().(ed25519.PubKey),
		ed25519.GenPrivKey().PubKey().(ed25519.PubKey),
	}
	vals := types.TMValidators{}
	for i, pk := range pks {
		vals = append(vals, types.TMValidator{
			CometValidator: &ctypes.Validator{Address: pk.Address(), PubKey: pk, VotingPower: 100},
			Index:          i,
		})
	}
	proposerAddr := vals[1].GetDisplayAddress()

	rdm := types.NewRoundDataMap()
	rdm.AddProposer(100, 0, proposerAddr)
	rdm.AddVote(100, 0, vals[0].GetDisplayAddress(), cmtproto.PrevoteType, ctypes.BlockID{})

	d := NewLastRoundTableData(3, true, false)
	d.SetTMValidators(vals, nil)
	d.SetRoundData(rdm)
	d.SetCurrentRound(100, 0)

	for row := 0; row < d.GetRowCount(); row++ {
		for col := 0; col < d.GetColumnCount(); col++ {
			cell := d.GetCell(row, col)
			if cell == nil {
				continue
			}
			bg := cell.BackgroundColor
			t.Logf("row=%d col=%d text=%q bg=%v", row, col, cell.Text, bg)
		}
	}

	idx := 1
	row := idx / 3
	col := idx % 3
	cell := d.GetCell(row, col)
	if cell == nil {
		t.Fatalf("proposer cell at row=%d col=%d is nil", row, col)
	}
	bg := cell.BackgroundColor
	if bg != tcell.ColorDimGray {
		t.Errorf("proposer cell bg = %v, want DimGray; cell text=%q", bg, cell.Text)
	}
	if !strings.Contains(cell.Text, "2") {
		t.Errorf("proposer cell should be validator 2, got text=%q", cell.Text)
	}
}
