package dashboard

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

func TestStateContractConcurrent(t *testing.T) {
	s := New()
	ts := httptest.NewServer(s)
	defer ts.Close()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 40; j++ {
				core.Sink(s.Consume).Emit(core.Event{Kind: "log", Text: "fixture"})
				r, e := http.Get(ts.URL + "/state")
				if e != nil {
					t.Error(e)
					return
				}
				var data map[string]any
				e = json.NewDecoder(r.Body).Decode(&data)
				r.Body.Close()
				if e != nil || len(data) != 21 {
					t.Errorf("contract: %v, keys %d", e, len(data))
				}
			}
		}()
	}
	wg.Wait()
	if len(s.state.Logs) != 200 {
		t.Fatal(len(s.state.Logs))
	}
	step := core.Step{Title: "next"}
	s.Consume(core.Event{Kind: "step", Step: &step})
	if s.state.AIAnswer != "" || s.state.EvalPassed != nil {
		t.Fatal(s.state)
	}
	r, e := http.Get(ts.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	b, e := io.ReadAll(r.Body)
	if e != nil || len(b) < 1000 {
		t.Fatal(e, len(b))
	}
}
func TestPortFallback(t *testing.T) {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if port > 65520 {
		t.Skip("port boundary")
	}
	s := New()
	if e = s.Start(port); e != nil {
		t.Fatal(e)
	}
	defer s.Close(context.Background())
	if s.URL == "http://"+ln.Addr().String()+"/" {
		t.Fatal("bound occupied port")
	}
}
