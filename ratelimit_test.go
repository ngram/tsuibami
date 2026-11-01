package tsuibami

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

// 予約した時刻どうしが min 以上離れていることと、予約した時刻より前に通さないことを確かめる。
// 実際に通った時刻の間隔は、タイマーの発火の遅れで縮みうるので比べない。
func TestRateGate(t *testing.T) {
	const min = 20 * time.Millisecond
	g := &rateGate{min: min}

	var (
		mu    sync.Mutex
		slots []time.Time
		wg    sync.WaitGroup
	)
	for range 5 {
		wg.Go(func() {
			slot, err := g.wait(context.Background())
			passed := time.Now()
			if err != nil {
				t.Error(err)
				return
			}
			if passed.Before(slot) {
				t.Errorf("予約した時刻 %v より前の %v に通った", slot, passed)
			}
			mu.Lock()
			slots = append(slots, slot)
			mu.Unlock()
		})
	}
	wg.Wait()

	slices.SortFunc(slots, func(a, b time.Time) int { return a.Compare(b) })
	for i := 1; i < len(slots); i++ {
		if d := slots[i].Sub(slots[i-1]); d < min {
			t.Errorf("予約した時刻 %d と %d の間が %v（%v 以上のはず）", i-1, i, d, min)
		}
	}
}

func TestRateGateCanceled(t *testing.T) {
	g := &rateGate{min: time.Hour}
	if _, err := g.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := g.wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v; want DeadlineExceeded", err)
	}
}
