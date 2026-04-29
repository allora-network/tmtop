package types

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	cmtbytes "github.com/cometbft/cometbft/libs/bytes"
)

type NetInfo struct {
	Listening bool     `json:"listening"`
	Listeners []string `json:"listeners"`
	NPeers    string   `json:"n_peers"`
	Peers     []Peer   `json:"peers"`
}

type Peer struct {
	NodeInfo         DefaultNodeInfo  `json:"node_info"`
	IsOutbound       bool             `json:"is_outbound"`
	ConnectionStatus ConnectionStatus `json:"connection_status"`
	RemoteIP         string           `json:"remote_ip"`
}

func (p Peer) URL() string {
	u, err := url.Parse(p.NodeInfo.Other.RPCAddress)
	if err != nil {
		return "http://" + p.RemoteIP + ":26657"
	}
	return "http" + "://" + p.RemoteIP + ":" + u.Port()
}

type DefaultNodeInfo struct {
	ProtocolVersion ProtocolVersion `json:"protocol_version"`

	// Authenticate
	DefaultNodeID ID     `json:"id"`          // authenticated identifier
	ListenAddr    string `json:"listen_addr"` // accepting incoming

	// Check compatibility.
	Network  string            `json:"network"`  // network/chain ID
	Version  string            `json:"version"`  // major.minor.revision
	Channels cmtbytes.HexBytes `json:"channels"` // channels this node knows about

	// ASCIIText fields
	Moniker string               `json:"moniker"` // arbitrary moniker
	Other   DefaultNodeInfoOther `json:"other"`   // other application specific data
}

type DefaultNodeInfoOther struct {
	TxIndex    string `json:"tx_index"`
	RPCAddress string `json:"rpc_address"`
}

type ProtocolVersion struct {
	P2P   int64 `json:"p2p"`
	Block int64 `json:"block"`
	App   int64 `json:"app"`
}

type ID string

// JSON keys inside ConnectionStatus / FlowStatus / ChannelStatus are
// PascalCase because CometBFT's libs/flowrate and ConnectionStatus
// types upstream don't define json tags, so the encoder uses Go field
// names verbatim. cmtjson matches keys case-sensitively, so the tags
// here must match exactly — case-insensitive fallback (which encoding/
// json provides) does not apply.
type ConnectionStatus struct {
	Duration    NanoDuration    `json:"Duration"`
	SendMonitor FlowStatus      `json:"SendMonitor"`
	RecvMonitor FlowStatus      `json:"RecvMonitor"`
	Channels    []ChannelStatus `json:"Channels"`
}

type ChannelStatus struct {
	ID                byte   `json:"ID"`
	SendQueueCapacity string `json:"SendQueueCapacity"`
	SendQueueSize     string `json:"SendQueueSize"`
	Priority          string `json:"Priority"`
	RecentlySent      string `json:"RecentlySent"`
}

type FlowStatus struct {
	Start    CustomTime   `json:"Start"`    // Transfer start time
	Bytes    ByteSize     `json:"Bytes"`    // Total number of bytes transferred
	Samples  ByteSize     `json:"Samples"`  // Total number of samples taken
	InstRate ByteSize     `json:"InstRate"` // Instantaneous transfer rate
	CurRate  ByteSize     `json:"CurRate"`  // Current transfer rate (EMA of InstRate)
	AvgRate  ByteSize     `json:"AvgRate"`  // Average transfer rate (Bytes / Duration)
	PeakRate ByteSize     `json:"PeakRate"` // Maximum instantaneous transfer rate
	BytesRem ByteSize     `json:"BytesRem"` // Number of bytes remaining in the transfer
	Duration NanoDuration `json:"Duration"` // Time period covered by the statistics
	Idle     NanoDuration `json:"Idle"`     // Time since the last transfer of at least 1 byte
	TimeRem  NanoDuration `json:"TimeRem"`  // Estimated time to completion
	Progress Percent      `json:"Progress"` // Overall transfer progress
	Active   bool         `json:"Active"`   // Flag indicating an active transfer
}

type NanoDuration time.Duration

func (nd *NanoDuration) UnmarshalJSON(b []byte) error {
	// CometBFT serializes int64 nanoseconds as a JSON string, so the
	// raw bytes look like "699569204788255" (quotes included). Accept
	// either the quoted or unquoted form.
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	nanos, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid duration: %v", err)
	}
	*nd = NanoDuration(time.Duration(nanos))
	return nil
}

// String returns the string representation of the duration.
func (nd NanoDuration) String() string {
	return time.Duration(nd).String()
}

type CustomTime struct {
	time.Time
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (ct *CustomTime) UnmarshalJSON(b []byte) error {
	// Remove quotes
	s := string(b)
	s = s[1 : len(s)-1]

	// Parse the time string
	t, err := time.Parse("2006-01-02T15:04:05.99Z", s)
	if err != nil {
		return err
	}

	ct.Time = t
	return nil
}

func (ct CustomTime) String() string {
	return ct.Time.Format("2006-01-02T15:04:05.99Z")
}

// Percent represents a percentage in increments of 1/1000th of a percent.
type Percent uint32

func (p Percent) Float() float64 {
	return float64(p) * 1e-3
}

func (p Percent) String() string {
	var buf [12]byte
	b := strconv.AppendUint(buf[:0], uint64(p)/1000, 10)
	n := len(b)
	b = strconv.AppendUint(b, 1000+uint64(p)%1000, 10)
	b[n] = '.'
	return string(append(b, '%'))
}

type ByteSize int64

const (
	_           = iota // ignore first value by assigning to blank identifier
	KB ByteSize = 1 << (10 * iota)
	MB
	GB
	TB
	PB
)

func (b ByteSize) String() string {
	switch {
	case b >= PB:
		return fmt.Sprintf("%.2fpb", float64(b)/float64(PB))
	case b >= TB:
		return fmt.Sprintf("%.2ftb", float64(b)/float64(TB))
	case b >= GB:
		return fmt.Sprintf("%.2fgb", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.2fmb", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.2fkb", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%db", b)
	}
}
