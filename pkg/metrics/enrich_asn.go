// pkg/metrics/enrich_asn.go                 (WS-I)
package metrics

import (
	"main/pkg/asn"
	"main/pkg/types"
)

func enrichASN(nh *NetworkHealth, s *types.State, a *asn.Lookup)             {}
func enrichASNRows(rows []ValidatorHealthRow, s *types.State, a *asn.Lookup) {}
