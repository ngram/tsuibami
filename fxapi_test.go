package tsuibami

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// countingServer は、受けた要求を数え、handler に応答させる。
func countingServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func TestFallback(t *testing.T) {
	public, publicHits := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"code":200,"message":"OK","tweet":{"id":"1","url":"https://x.com/u/status/1","text":"公開版"}}`))
	})

	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	tests := []struct {
		name         string
		local        func(w http.ResponseWriter, r *http.Request) // nil ならつながらない
		wantFallback bool
		wantErr      bool
	}{
		{"つながらない", nil, true, false},
		{"404", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":404,"message":"NOT_FOUND"}`))
		}, true, false},
		{"500", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"code":500,"message":"API_FAIL"}`))
		}, false, true},
		{"401", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"code":401,"message":"UNAUTHORIZED"}`))
		}, false, true},
		{"リダイレクトは追わない", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://example.com/", http.StatusFound)
		}, false, true},
		{"成功", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"code":200,"message":"OK","tweet":{"id":"1","url":"https://x.com/u/status/1","text":"自分のFxEmbed"}}`))
		}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := closedURL
			if tt.local != nil {
				local, _ := countingServer(t, tt.local)
				base = local.URL
			}
			c := New(WithBaseURL(base), WithFallbackURL(public.URL), WithInterval(0))
			before := publicHits.Load()
			p, err := c.Post(context.Background(), "https://x.com/u/status/1")

			if got := publicHits.Load() - before; (got > 0) != tt.wantFallback {
				t.Errorf("公開版への要求 = %d; 取り直すはず = %v", got, tt.wantFallback)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v; wantErr = %v", err, tt.wantErr)
			}
			if err == nil && tt.wantFallback && p.Text != "公開版" {
				t.Errorf("Text = %q", p.Text)
			}
		})
	}
}

func TestFallbackAPIErrorCode(t *testing.T) {
	local, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code":500,"message":"API_FAIL"}`))
	})
	_, err := New(WithBaseURL(local.URL), WithFallbackURL(""), WithInterval(0)).Post(context.Background(), "1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 500 || apiErr.Message != "API_FAIL" {
		t.Errorf("err = %v; want APIError{500, API_FAIL}", err)
	}
}

// 呼び出し元が ctx をキャンセルしたときは、公開版で取り直さない。
func TestNoFallbackOnCancel(t *testing.T) {
	arrived := make(chan struct{})
	local, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-r.Context().Done()
	})
	public, publicHits := countingServer(t, func(w http.ResponseWriter, r *http.Request) {})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-arrived
		cancel()
	}()
	_, err := New(WithBaseURL(local.URL), WithFallbackURL(public.URL), WithInterval(0)).Post(ctx, "1")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want context.Canceled", err)
	}
	if n := publicHits.Load(); n != 0 {
		t.Errorf("公開版への要求 = %d; want 0", n)
	}
}
