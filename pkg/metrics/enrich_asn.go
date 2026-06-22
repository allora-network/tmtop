// pkg/metrics/enrich_asn.go                 (WS-I)
package metrics

import (
	"sort"

	"main/pkg/asn"
	"main/pkg/types"
)

const topASNCount = 5

// enrichASN populates nh.TopASNs with the top hosting providers by combined
// voting power across the active validator set.
//
// The join is: validator hex-address → RPC.ValidatorAddress → RPC.IP → ASN.
// Only validators whose IP is discoverable via the peer/RPC graph are counted;
// the rest are silently omitted (coverage depends on topology crawl depth).
func enrichASN(nh *NetworkHealth, s *types.State, lookup *asn.Lookup) {
	if lookup == nil {
		return
	}

	addrToIP := buildAddrToIPMap(s)

	type asnAccum struct {
		asn        uint32
		org        string
		validators int
		powerPct   float64
	}
	asnMap := make(map[uint32]*asnAccum)

	validators := s.GetTMValidators()
	for _, v := range validators {
		addr := v.GetDisplayAddress()
		ip, ok := addrToIP[addr]
		if !ok || ip == "" {
			continue
		}
		asnNum, org, found := lookup.Lookup(ip)
		if !found {
			continue
		}
		pct := 0.0
		if v.VotingPowerPercent != nil {
			pct, _ = v.VotingPowerPercent.Float64()
		}
		acc, exists := asnMap[asnNum]
		if !exists {
			acc = &asnAccum{asn: asnNum, org: org}
			asnMap[asnNum] = acc
		}
		acc.validators++
		acc.powerPct += pct
	}

	// Collect, sort descending by power, take top N.
	shares := make([]ASNShare, 0, len(asnMap))
	for _, acc := range asnMap {
		shares = append(shares, ASNShare{
			ASN:         acc.asn,
			Description: acc.org,
			Validators:  acc.validators,
			PowerPct:    acc.powerPct,
		})
	}
	sort.Slice(shares, func(i, j int) bool {
		if shares[i].PowerPct != shares[j].PowerPct {
			return shares[i].PowerPct > shares[j].PowerPct
		}
		return shares[i].ASN < shares[j].ASN // stable tiebreak
	})
	if len(shares) > topASNCount {
		shares = shares[:topASNCount]
	}
	nh.TopASNs = shares
}

// enrichASNRows annotates each ValidatorHealthRow with the ASN and organisation
// string for that validator's IP, when discoverable.  Rows without a known IP
// are left with ASN=0 and ASNOrg="" (renders as blank in the UI).
func enrichASNRows(rows []ValidatorHealthRow, s *types.State, lookup *asn.Lookup) {
	if lookup == nil {
		return
	}

	addrToIP := buildAddrToIPMap(s)

	for i := range rows {
		ip, ok := addrToIP[rows[i].Address]
		if !ok || ip == "" {
			continue
		}
		asnNum, org, found := lookup.Lookup(ip)
		if !found {
			continue
		}
		rows[i].ASN = asnNum
		rows[i].ASNOrg = org
	}
}

// buildAddrToIPMap builds a hex-validator-address → IP map from the known RPC
// peer graph.  Each types.RPC entry may carry a ValidatorAddress linking the
// peer's P2P node to a consensus validator.
func buildAddrToIPMap(s *types.State) map[string]string {
	rpcs := s.KnownRPCs()
	m := make(map[string]string, rpcs.Len())
	rpcs.Range(func(_ string, rpc types.RPC) {
		if rpc.ValidatorAddress != "" && rpc.IP != "" {
			m[rpc.ValidatorAddress] = rpc.IP
		}
	})
	return m
}
