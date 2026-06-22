package metrics

import (
	"testing"
	"time"

	"main/pkg/types"
	"github.com/rs/zerolog"
)

func TestBuilderBuildEmpty(t *testing.T) {
	clock := time.Unix(1700000000, 0)
	b := NewBuilder(zerolog.Nop(), func() time.Time { return clock }, nil, nil, 1000)
	st := types.NewState("", zerolog.Nop()) // NewState requires (firstRPC string, logger zerolog.Logger)
	nh, rows := b.Build(st)
	if nh == nil {
		t.Fatal("nil NetworkHealth")
	}
	if rows == nil {
		t.Fatal("nil rows (want empty slice)")
	}
}
