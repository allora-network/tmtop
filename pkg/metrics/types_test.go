package metrics

import "testing"

func TestNetworkHealthZeroValue(t *testing.T) {
	var nh NetworkHealth
	if nh.ChainHalted || nh.Nakamoto33 != 0 || nh.MempoolKnown {
		t.Fatalf("zero value should be empty: %+v", nh)
	}
}
