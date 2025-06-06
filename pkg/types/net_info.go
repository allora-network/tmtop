package types

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"time"

	cmtbytes "github.com/cometbft/cometbft/libs/bytes"
)

type NetInfo struct {
	Listening bool     `mapstructure:"listening"`
	Listeners []string `mapstructure:"listeners"`
	NPeers    string   `mapstructure:"n_peers"`
	Peers     []Peer   `mapstructure:"peers"`
}

type Peer struct {
	NodeInfo         DefaultNodeInfo  `mapstructure:"node_info"`
	IsOutbound       bool             `mapstructure:"is_outbound"`
	ConnectionStatus ConnectionStatus `mapstructure:"connection_status"`
	RemoteIP         string           `mapstructure:"remote_ip"`
}

func (p Peer) URL() string {
	u, err := url.Parse(p.NodeInfo.Other.RPCAddress)
	if err != nil {
		return "http://" + p.RemoteIP + ":26657"
	}
	return "http" + "://" + p.RemoteIP + ":" + u.Port()
}

type DefaultNodeInfo struct {
	ProtocolVersion ProtocolVersion `mapstructure:"protocol_version"`

	// Authenticate
	// TODO: replace with NetAddress
	DefaultNodeID ID     `mapstructure:"id"`          // authenticated identifier
	ListenAddr    string `mapstructure:"listen_addr"` // accepting incoming

	// Check compatibility.
	// Channels are HexBytes so easier to read as JSON
	Network  string            `mapstructure:"network"`  // network/chain ID
	Version  string            `mapstructure:"version"`  // major.minor.revision
	Channels cmtbytes.HexBytes `mapstructure:"channels"` // channels this node knows about

	// ASCIIText fields
	Moniker string               `mapstructure:"moniker"` // arbitrary moniker
	Other   DefaultNodeInfoOther `mapstructure:"other"`   // other application specific data
}

type DefaultNodeInfoOther struct {
	TxIndex    string `mapstructure:"tx_index"`
	RPCAddress string `mapstructure:"rpc_address"`
}

type ProtocolVersion struct {
	P2P   int64 `mapstructure:"p2p"`
	Block int64 `mapstructure:"block"`
	App   int64 `mapstructure:"app"`
}

type ID string

type ConnectionStatus struct {
	Duration    NanoDuration    `json:"duration"`
	SendMonitor FlowStatus      `json:"send_monitor"`
	RecvMonitor FlowStatus      `json:"recv_monitor"`
	Channels    []ChannelStatus `json:"channels"`
}

type ChannelStatus struct {
	ID                byte   `json:"id"`
	SendQueueCapacity string `json:"send_queue_capacity"`
	SendQueueSize     string `json:"send_queue_size"`
	Priority          string `json:"priority"`
	RecentlySent      string `json:"recently_sent"`
}

type FlowStatus struct {
	Start    CustomTime   `json:"start"`     // Transfer start time
	Bytes    ByteSize     `json:"bytes"`     // Total number of bytes transferred
	Samples  ByteSize     `json:"samples"`   // Total number of samples taken
	InstRate ByteSize     `json:"inst_rate"` // Instantaneous transfer rate
	CurRate  ByteSize     `json:"cur_rate"`  // Current transfer rate (EMA of InstRate)
	AvgRate  ByteSize     `json:"avg_rate"`  // Average transfer rate (Bytes / Duration)
	PeakRate ByteSize     `json:"peak_rate"` // Maximum instantaneous transfer rate
	BytesRem ByteSize     `json:"bytes_rem"` // Number of bytes remaining in the transfer
	Duration NanoDuration `json:"duration"`  // Time period covered by the statistics
	Idle     NanoDuration `json:"idle"`      // Time since the last transfer of at least 1 byte
	TimeRem  NanoDuration `json:"time_rem"`  // Estimated time to completion
	Progress Percent      `json:"progress"`  // Overall transfer progress
	Active   bool         `json:"active"`    // Flag indicating an active transfer
}

type NanoDuration time.Duration

func (nd *NanoDuration) UnmarshalJSON(b []byte) error {
	// Remove quotes from the string
	// s := string(b)
	// s = s[1 : len(s)-1]

	// Parse the string as an int64
	nanos, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid duration: %v", err)
	}

	// Convert nanoseconds to time.Duration
	*nd = NanoDuration(time.Duration(nanos))
	return nil
}

// String returns the string representation of the duration
func (nd NanoDuration) String() string {
	return time.Duration(nd).String()
}

type CustomTime struct {
	time.Time
}

// UnmarshalJSON implements the json.Unmarshaler interface
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

func StringToCustomTimeHookFunc(
	f reflect.Type,
	t reflect.Type,
	data any,
) (any, error) {
	if f.Kind() != reflect.String {
		return data, nil
	} else if t != reflect.TypeOf(CustomTime{}) {
		return data, nil
	}

	str := data.(string)
	result, err := time.Parse("2006-01-02T15:04:05.99Z", str)
	if err != nil {
		return nil, fmt.Errorf("failed to parse time: %v", err)
	}
	return CustomTime{Time: result}, nil
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
