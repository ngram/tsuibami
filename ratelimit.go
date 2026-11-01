package tsuibami

import (
	"context"
	"sync"
	"time"
)

// rateGate は次に通過してよい時刻を持ち、そこまで呼び出し側を待たせる。
//
// 待つ前に自分の通過時刻を予約してロックを手放すため、複数の goroutine が同時に来ても
// 待ち時間が重ならず、min 間隔ずつ順に通る。予約するのは時刻であり、実際に通過した時刻ではない。
// タイマーの発火が遅れると、実際の通過の間隔は min よりわずかに縮むことがある。
type rateGate struct {
	mu   sync.Mutex
	min  time.Duration
	next time.Time // 次に通過してよい時刻
}

// wait はゲートが開くまで待ち、予約した時刻を返す。ctx がキャンセルされたらその error を返す。
// 最初の呼び出しは待たない。
//
// キャンセルで抜けた場合も予約した時刻は戻さない。次の呼び出しが最大 min だけ余分に
// 待つだけで、制限としては安全側に外れるため、複雑な巻き戻しは持たない。
func (g *rateGate) wait(ctx context.Context) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}

	g.mu.Lock()
	slot := g.next
	if now := time.Now(); slot.Before(now) {
		slot = now
	}
	g.next = slot.Add(g.min)
	g.mu.Unlock()

	d := time.Until(slot)
	if d <= 0 {
		return slot, nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return time.Time{}, ctx.Err()
	case <-t.C:
		return slot, nil
	}
}
