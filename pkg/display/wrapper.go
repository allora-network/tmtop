package display

import (
	"fmt"
	configPkg "main/pkg/config"
	"main/pkg/types"
	"main/static"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rs/zerolog"
)

const (
	ModeLastRound = iota
	ModeAllRounds
	ModeNetInfo
)

const (
	DefaultColumnsCount = 3
	RowsAmount          = 10
	DebugBlockHeight    = 8
	DefaultMode         = ModeAllRounds
)

type Wrapper struct {
	ConsensusInfoTextView *tview.TextView
	ChainInfoTextView     *tview.TextView
	ProgressTextView      *tview.TextView
	DebugTextView         *tview.TextView
	LastRoundTable        *tview.Table
	LastRoundTableData    *LastRoundTableData
	AllRoundsTable        *tview.Table
	AllRoundsTableData    *AllRoundsTableData
	NetInfoTable          *tview.Table
	NetInfoTableData      *NetInfoTableData
	RPCsTable             *tview.Table
	RPCsTableData         *RPCsTableData
	Grid                  *tview.Grid
	Pages                 *tview.Pages
	App                   *tview.Application
	HelpModal             *tview.Modal

	InfoBlockWidth int
	ColumnsCount   int
	Mode           int

	DebugEnabled bool

	State  *types.State
	Logger zerolog.Logger

	PauseChannel chan bool
	IsPaused     bool

	IsRPCListDisplayed bool
	IsHelpDisplayed    bool

	DisableEmojis bool
	Transpose     bool
	Timezone      *time.Location
}

func NewWrapper(
	config *configPkg.Config,
	state *types.State,
	logger zerolog.Logger,
	pauseChannel chan bool,
	appVersion string,
) *Wrapper {
	lastRoundTableData := NewLastRoundTableData(DefaultColumnsCount, config.DisableEmojis, false)
	allRoundsTableData := NewAllRoundsTableData(config.DisableEmojis, false)
	netInfoTableData := NewNetInfoTableData()
	rpcsTableData := NewRPCsTableData()

	helpTextBytes, _ := static.TemplatesFs.ReadFile("help.txt")
	helpText := strings.ReplaceAll(string(helpTextBytes), "{{ Version }}", appVersion)

	lastRoundTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false).
		SetContent(lastRoundTableData)

	allRoundsTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false).
		SetContent(allRoundsTableData).
		SetFixed(1, 1)

	netInfoTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false).
		SetFixed(2, 0).
		SetEvaluateAllRows(true).
		SetContent(netInfoTableData)

	rpcsTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetEvaluateAllRows(true).
		SetContent(rpcsTableData)
	rpcsTable.SetTitle("RPCs").SetTitleAlign(tview.AlignCenter).SetBorder(true)

	consensusInfoTextView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true)

	chainInfoTextView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true)

	progressTextView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true)

	debugTextView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true)

	helpModal := tview.NewModal().SetText(helpText)

	grid := tview.NewGrid().
		SetRows(0, 0, 0, 0, 0, 0, 0, 0, 0, 0).
		SetColumns(0, 0, 0, 0, 0, 0).
		SetBorders(true)

	pages := tview.NewPages().AddPage("grid", grid, true, true)

	app := tview.NewApplication().SetRoot(pages, true).SetFocus(lastRoundTable)

	return &Wrapper{
		State:                 state,
		ChainInfoTextView:     chainInfoTextView,
		ConsensusInfoTextView: consensusInfoTextView,
		ProgressTextView:      progressTextView,
		DebugTextView:         debugTextView,
		LastRoundTable:        lastRoundTable,
		LastRoundTableData:    lastRoundTableData,
		AllRoundsTable:        allRoundsTable,
		AllRoundsTableData:    allRoundsTableData,
		NetInfoTable:          netInfoTable,
		NetInfoTableData:      netInfoTableData,
		RPCsTable:             rpcsTable,
		RPCsTableData:         rpcsTableData,
		HelpModal:             helpModal,
		Grid:                  grid,
		Pages:                 pages,
		App:                   app,
		Logger:                logger.With().Str("component", "display_wrapper").Logger(),
		DebugEnabled:          false,
		InfoBlockWidth:        2,
		ColumnsCount:          DefaultColumnsCount,
		Mode:                  DefaultMode,
		PauseChannel:          pauseChannel,
		IsPaused:              false,
		IsHelpDisplayed:       false,
		DisableEmojis:         config.DisableEmojis,
		Transpose:             false,
		Timezone:              config.Timezone,
	}
}

func (w *Wrapper) Start() {
	w.RPCsTable.SetSelectedFunc(func(row, col int) {
		rpc, ok := w.State.RPCAtIndex(row)
		if ok {
			w.State.SetCurrentRPCURL(rpc.URL)
		}
		// w.State.Clear()
		// w.SetState(w.State)
		w.ToggleRPCList()
	})

	w.App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Key() == tcell.KeyCtrlC {
			w.App.Stop()
			return nil
		}

		if event.Rune() == 'd' {
			w.ToggleDebug()
		}

		if event.Rune() == 'b' {
			w.ChangeInfoBlockHeight(true)
		}

		if event.Rune() == 's' {
			w.ChangeInfoBlockHeight(false)
		}

		if event.Rune() == 'h' {
			w.ToggleHelp()
		}

		if event.Rune() == 'r' {
			w.ToggleRPCList()
		}

		if event.Rune() == 'm' {
			w.ChangeColumnsCount(true)
		}

		if event.Rune() == 'l' {
			w.ChangeColumnsCount(false)
		}

		if event.Rune() == 't' {
			w.Transpose = !w.Transpose
			w.LastRoundTableData.SetTranspose(w.Transpose)
			w.AllRoundsTableData.SetTranspose(w.Transpose)
		}

		if event.Rune() == 'p' {
			w.IsPaused = !w.IsPaused
			w.PauseChannel <- w.IsPaused
		}

		if event.Key() == tcell.KeyTAB {
			w.ChangeMode()
		}

		return event
	})

	w.Grid.SetBackgroundColor(tcell.ColorDefault)
	w.LastRoundTable.SetBackgroundColor(tcell.ColorDefault)
	w.AllRoundsTable.SetBackgroundColor(tcell.ColorDefault)
	w.NetInfoTable.SetBackgroundColor(tcell.ColorDefault)
	w.RPCsTable.SetBackgroundColor(tcell.ColorSteelBlue)
	w.ChainInfoTextView.SetBackgroundColor(tcell.ColorDefault)
	w.ConsensusInfoTextView.SetBackgroundColor(tcell.ColorDefault)
	w.ProgressTextView.SetBackgroundColor(tcell.ColorDefault)
	w.DebugTextView.SetBackgroundColor(tcell.ColorDefault)

	w.Redraw()

	_, _ = fmt.Fprint(w.ChainInfoTextView, "Loading...")
	_, _ = fmt.Fprint(w.ConsensusInfoTextView, "Loading...")
	_, _ = fmt.Fprint(w.ProgressTextView, "Loading...")

	w.App.SetAfterDrawFunc(func(screen tcell.Screen) {
		_, _, width, _ := w.LastRoundTable.GetInnerRect()
		columns := width / 50
		w.LastRoundTableData.SetColumnsCount(columns)

		w.App.SetAfterDrawFunc(nil)
	})

	if err := w.App.Run(); err != nil {
		w.Logger.Error().Err(err).Msg("Could not draw screen")
		w.cleanup()
	}
}

func (w *Wrapper) ToggleDebug() {
	w.DebugEnabled = !w.DebugEnabled
	w.Redraw()
}

func (w *Wrapper) ToggleRPCList() {
	w.IsRPCListDisplayed = !w.IsRPCListDisplayed
	w.Redraw()
}

func (w *Wrapper) ToggleHelp() {
	w.IsHelpDisplayed = !w.IsHelpDisplayed
	w.Redraw()
}

func (w *Wrapper) SetState(state *types.State) {
	w.App.QueueUpdateDraw(func() {
		w.State = state

		// Use TMValidators
		w.LastRoundTableData.SetTMValidators(state.GetTMValidators(), state.ConsensusStateError)
		if w.AllRoundsTableData != nil {
			w.AllRoundsTableData.SetTMValidators(state.GetTMValidators(), state.Height)
			w.AllRoundsTableData.SetRoundData(state.VotesByRound)
			w.AllRoundsTableData.Update()
		}
		w.NetInfoTableData.SetNetInfo(state.NetInfo)
		w.RPCsTableData.SetKnownRPCs(state.KnownRPCs().Values())

		w.ConsensusInfoTextView.Clear()
		w.ChainInfoTextView.Clear()
		w.ProgressTextView.Clear()
		_, _ = fmt.Fprint(w.ConsensusInfoTextView, state.SerializeConsensus(w.Timezone))
		_, _ = fmt.Fprint(w.ChainInfoTextView, state.SerializeChainInfo(w.Timezone))

		_, _, width, height := w.ConsensusInfoTextView.GetInnerRect()
		_, _ = fmt.Fprint(w.ProgressTextView, state.SerializePrevotesProgressbar(width, height/2))
		_, _ = fmt.Fprint(w.ProgressTextView, "\n")
		_, _ = fmt.Fprint(w.ProgressTextView, state.SerializePrecommitsProgressbar(width, height/2))

		// w.App.Draw()
	})
}

func (w *Wrapper) DebugText(text string) {
	_, _ = fmt.Fprint(w.DebugTextView, text+"\n")
	w.DebugTextView.ScrollToEnd()
}

func (w *Wrapper) ChangeInfoBlockHeight(increase bool) {
	if increase && w.InfoBlockWidth+1 <= RowsAmount-DebugBlockHeight-1 {
		w.InfoBlockWidth++
	} else if !increase && w.InfoBlockWidth-1 >= 1 {
		w.InfoBlockWidth--
	}

	w.Redraw()
}

func (w *Wrapper) ChangeColumnsCount(increase bool) {
	if increase {
		w.ColumnsCount++
	} else if !increase && w.ColumnsCount-1 >= 1 {
		w.ColumnsCount--
	}

	w.LastRoundTableData.SetColumnsCount(w.ColumnsCount)

	w.Redraw()
}

func (w *Wrapper) ChangeMode() {
	switch w.Mode {
	case ModeAllRounds:
		w.Mode = ModeNetInfo
	case ModeNetInfo:
		w.Mode = ModeLastRound
	case ModeLastRound:
		w.Mode = ModeAllRounds
	default:
		w.Mode = ModeAllRounds
	}

	w.Redraw()
}

func (w *Wrapper) Redraw() {
	table := w.LastRoundTable
	if w.Mode == ModeAllRounds {
		table = w.AllRoundsTable
	} else if w.Mode == ModeNetInfo {
		table = w.NetInfoTable
	}

	w.Grid.RemoveItem(w.ConsensusInfoTextView)
	w.Grid.RemoveItem(w.ChainInfoTextView)
	w.Grid.RemoveItem(w.ProgressTextView)
	w.Grid.RemoveItem(w.LastRoundTable)
	w.Grid.RemoveItem(w.AllRoundsTable)
	w.Grid.RemoveItem(w.DebugTextView)

	w.Grid.AddItem(w.ConsensusInfoTextView, 0, 0, w.InfoBlockWidth, 3, 1, 1, false)
	w.Grid.AddItem(w.ChainInfoTextView, 0, 3, w.InfoBlockWidth, 2, 1, 1, false)
	w.Grid.AddItem(w.ProgressTextView, 0, 5, w.InfoBlockWidth, 1, 1, 1, false)

	if w.DebugEnabled {
		w.Grid.AddItem(
			table,
			w.InfoBlockWidth,
			0,
			RowsAmount-w.InfoBlockWidth-DebugBlockHeight,
			6,
			0,
			0,
			false,
		)
		w.Grid.AddItem(
			w.DebugTextView,
			RowsAmount-DebugBlockHeight,
			0,
			DebugBlockHeight,
			6,
			0,
			0,
			false,
		)
	} else {
		w.Grid.AddItem(
			table,
			w.InfoBlockWidth,
			0,
			RowsAmount-w.InfoBlockWidth,
			6,
			0,
			0,
			false,
		)
	}

	w.App.SetFocus(table)

	if w.IsHelpDisplayed {
		w.Pages.AddPage("modal", w.HelpModal, true, true)
	} else {
		w.Pages.RemovePage("modal")
	}

	if w.IsRPCListDisplayed {
		_, _, pwidth, pheight := w.Pages.GetRect()
		width := int(float64(pwidth) / 2.5)
		height := pheight - 8
		x := (pwidth - width) / 2
		y := (pheight - height) / 2
		w.RPCsTable.SetRect(x, y, width, height)
		w.Pages.AddPage("rpclist", w.RPCsTable, false, true)
		w.App.SetFocus(w.RPCsTable)
	} else {
		w.Pages.RemovePage("rpclist")
		w.App.SetFocus(table)
	}
}

// cleanup ensures proper terminal state restoration
func (w *Wrapper) cleanup() {
	if w.App != nil {
		// Stop the application gracefully
		w.App.Stop()
	}

	// Additional terminal cleanup
	w.restoreTerminal()
}

// restoreTerminal restores terminal state
func (w *Wrapper) restoreTerminal() {
	// Send terminal reset sequences to restore state
	fmt.Print("\033[?25h")   // Show cursor
	fmt.Print("\033[0m")     // Reset colors
	fmt.Print("\033[2J")     // Clear screen
	fmt.Print("\033[H")      // Move cursor to top-left
	fmt.Print("\033[?1049l") // Exit alternate screen buffer
}
