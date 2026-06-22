package metrics

import (
	"math"
	"testing"
)

func powers(vals ...int64) []int64 { return vals }

func TestNakamoto(t *testing.T) {
	// total=100; sorted desc 40,30,20,10. >1/3=33.3 ⇒ 40 alone (1). >2/3=66.6 ⇒ 40+30=70 (2).
	got33, got66 := nakamoto(powers(10, 40, 20, 30))
	if got33 != 1 || got66 != 2 {
		t.Fatalf("nakamoto = %d,%d want 1,2", got33, got66)
	}
}

func TestGiniEqual(t *testing.T) {
	if g := gini(powers(10, 10, 10, 10)); math.Abs(g) > 1e-9 {
		t.Fatalf("gini(equal) = %v want 0", g)
	}
}

func TestGiniSkew(t *testing.T) {
	g := gini(powers(0, 0, 0, 100))
	if g < 0.7 { // highly concentrated ⇒ near (n-1)/n = 0.75
		t.Fatalf("gini(skew) = %v want >0.7", g)
	}
}

func TestNakamotoEmpty(t *testing.T) {
	n33, n66 := nakamoto(powers())
	if n33 != 0 || n66 != 0 {
		t.Fatalf("nakamoto(empty) = %d,%d want 0,0", n33, n66)
	}
}

func TestNakamotoSingle(t *testing.T) {
	n33, n66 := nakamoto(powers(100))
	if n33 != 1 || n66 != 1 {
		t.Fatalf("nakamoto(single) = %d,%d want 1,1", n33, n66)
	}
}

func TestGiniEmpty(t *testing.T) {
	if g := gini(powers()); g != 0 {
		t.Fatalf("gini(empty) = %v want 0", g)
	}
}

func TestTopNShare(t *testing.T) {
	// 4 validators with equal power: top-10 captures all 4 → 100%
	got := topNShare(powers(25, 25, 25, 25), 10)
	if math.Abs(got-100.0) > 1e-9 {
		t.Fatalf("topNShare(equal,10) = %v want 100", got)
	}

	// top 1 of [40,30,20,10] = 40/100 = 40%
	got2 := topNShare(powers(10, 40, 20, 30), 1)
	if math.Abs(got2-40.0) > 1e-9 {
		t.Fatalf("topNShare([10,40,20,30],1) = %v want 40", got2)
	}
}
