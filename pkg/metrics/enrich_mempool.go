// pkg/metrics/enrich_mempool.go             (WS-D)
package metrics

import "main/pkg/types"

func enrichMempool(nh *NetworkHealth, s *types.State) {
	txs, bytes, known := s.GetMempool()
	nh.MempoolTxs, nh.MempoolBytes, nh.MempoolKnown = txs, bytes, known
}
