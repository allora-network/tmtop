package crawler

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"main/pkg/fetcher"
	"main/pkg/types"

	butils "github.com/brynbellomy/go-utils"
	"github.com/rs/zerolog"
)

// rescanInterval is how often the crawler re-queues every known RPC.
const rescanInterval = 15 * time.Second

// Crawler discovers RPC peers starting from a seed URL. It drains a
// mailbox of URLs to probe, writes results back into State, and
// re-queues known URLs on a timer so the topology stays fresh.
type Crawler struct {
	seed     string
	fetcher  fetcher.Fetcher
	state    *types.State
	logger   zerolog.Logger
	chStop   <-chan struct{}
	mb       *butils.Mailbox[string]
	lastSeen map[string]time.Time
}

// New builds a Crawler. chStop is the parent's stop channel — the
// crawler does not own its lifecycle, it exits cleanly when chStop
// closes.
func New(
	seed string,
	fetcher fetcher.Fetcher,
	state *types.State,
	logger zerolog.Logger,
	chStop <-chan struct{},
) *Crawler {
	return &Crawler{
		seed:     seed,
		fetcher:  fetcher,
		state:    state,
		logger:   logger.With().Str("component", "crawler").Logger(),
		chStop:   chStop,
		mb:       butils.NewMailbox[string](1000),
		lastSeen: make(map[string]time.Time),
	}
}

// Run blocks until chStop closes. Call it inside a tracked goroutine.
func (c *Crawler) Run() {
	c.mb.Deliver(c.seed)

	ticker := time.NewTicker(rescanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.chStop:
			return

		case <-c.mb.Notify():
			var wg sync.WaitGroup
			for _, url := range c.mb.RetrieveAll() {
				if last, ok := c.lastSeen[url]; ok && time.Since(last) < rescanInterval {
					continue
				}
				c.lastSeen[url] = time.Now()

				wg.Add(1)
				go func() {
					defer wg.Done()
					c.fetchOne(url)
				}()
			}
			wg.Wait()

		case <-ticker.C:
			for _, rpc := range c.state.KnownRPCs().Iter() {
				if time.Since(c.lastSeen[rpc.URL]) >= rescanInterval {
					c.mb.Deliver(rpc.URL)
				}
			}
		}
	}
}

func (c *Crawler) fetchOne(rpcURL string) {
	netInfo, err := c.fetcher.GetNetInfo(rpcURL)
	if err != nil {
		c.logger.Error().Err(err).Msg(fmt.Sprintf("error getting /net_info from %s", rpcURL))
		return
	}

	status, err := c.fetcher.GetCometNodeStatus(rpcURL)
	if err != nil {
		c.logger.Error().Err(err).Msg(fmt.Sprintf("error getting /status from %s", rpcURL))
		return
	}

	var rpc types.RPC
	if known, ok := c.state.KnownRPCByURL(rpcURL); ok {
		rpc = known
	}
	rpc.ID = string(status.NodeInfo.ID())
	rpc.URL = rpcURL
	rpc.Moniker = status.NodeInfo.Moniker
	rpc.ValidatorAddress = status.ValidatorInfo.Address.String()

	for _, cv := range c.state.GetTMValidators() {
		// CosmosValidator/ConsensusPubkey can be absent on validators
		// where the Comet/Cosmos merge had no Cosmos-side match
		// (chains during genesis fallback, or partial responses) —
		// skip them rather than panicking on .Address().
		if cv.CosmosValidator == nil || cv.CosmosValidator.ConsensusPubkey == nil {
			continue
		}
		if strings.EqualFold(cv.CosmosValidator.ConsensusPubkey.Address().String(), rpc.ValidatorAddress) {
			rpc.ValidatorMoniker = cv.CosmosValidator.Moniker
			break
		}
	}

	c.state.AddKnownRPC(rpc)
	c.state.AddRPCPeers(rpcURL, netInfo.Peers)

	for _, peer := range netInfo.Peers {
		var peerRPC types.RPC
		if known, ok := c.state.KnownRPCByURL(peer.URL()); ok {
			peerRPC = known
		}
		peerRPC.ID = string(peer.NodeInfo.DefaultNodeID)
		peerRPC.IP = peer.RemoteIP
		peerRPC.URL = peer.URL()
		peerRPC.Moniker = peer.NodeInfo.Moniker
		c.state.AddKnownRPC(peerRPC)

		c.mb.Deliver(peer.URL())
	}
}
