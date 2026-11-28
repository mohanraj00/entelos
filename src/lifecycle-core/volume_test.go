package lifecycle

// TestGitStoreVolume measures the Git store as the count of receipts grows
// (#164). It runs only if ENTELOS_STORE_VOLUME names an output directory.
// See docs/SUPPORT.md for the method and the result.
//
//	ENTELOS_STORE_VOLUME=<dir> go test -run TestGitStoreVolume -timeout 0 .
//
// ENTELOS_STORE_VOLUME_MAX sets the count of receipts (default 20000).
// ENTELOS_STORE_VOLUME_MAX_BYTES stops the growth when the store directory
// is larger (default 4 GiB). The store grows fast, so check the free disk
// space first.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"
)

type volumeRun struct {
	t       *testing.T
	path    string
	store   *GitStore
	runner  Runner
	cursor  any
	keys    int
	first   Object // the first invoke request, for the retry
	written int
}

func (r *volumeRun) host(actor, role string, store Store) *Host {
	r.t.Helper()
	context := Object{"actor": actor, "roles": Array{role}, "scope": "demo", "runner_actor": "fixture-runner",
		"worker": "worker-1", "functions": Array{"record.visible"}}
	h, err := NewHostWithRunner(store, &fakeEngine{build: engineBuild}, r.runner, context)
	if err != nil {
		r.t.Fatal(err)
	}
	return h
}

// write sends one write request at the current cursor.
func (r *volumeRun) write(h *Host, op string, fields Object) (Object, error) {
	r.keys++
	q := Object{"op": op, "expected_revision": r.cursor, "request_key": "v-" + strconv.Itoa(r.keys)}
	for k, v := range fields {
		q[k] = v
	}
	out, err := h.Handle(q)
	if err != nil {
		return nil, err
	}
	r.cursor = out.(Object)["cursor"]
	return out.(Object)["result"].(Object), nil
}

func (r *volumeRun) must(actor, role, op string, fields Object) Object {
	r.t.Helper()
	out, err := r.write(r.host(actor, role, r.store), op, fields)
	if err != nil {
		r.t.Fatalf("%s: %v", op, err)
	}
	return out
}

func invokeFields(n int) Object {
	return Object{"function": "record.visible", "date": "2026-11-15", "expected_bundle_ref": bundleRefOf("pass"),
		"inputs": Object{"state": "ready-" + strconv.Itoa(n)}}
}

func dirSize(t *testing.T, dir string) int64 {
	t.Helper()
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		info, err := d.Info()
		if err == nil {
			total += info.Size()
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return total
}

func quantiles(d []time.Duration) (p50, p95, max float64) {
	if len(d) == 0 {
		return 0, 0, 0
	}
	s := append([]time.Duration{}, d...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	ms := func(x time.Duration) float64 { return float64(x.Microseconds()) / 1000 }
	return ms(s[len(s)/2]), ms(s[len(s)*95/100]), ms(s[len(s)-1])
}

func timed(f func() error) (float64, string) {
	start := time.Now()
	err := f()
	code := "ok"
	if err != nil {
		var fault *Fault
		if errors.As(err, &fault) {
			code = fault.Code
		} else {
			code = err.Error()
		}
	}
	return float64(time.Since(start).Microseconds()) / 1000, code
}

func TestGitStoreVolume(t *testing.T) {
	out := os.Getenv("ENTELOS_STORE_VOLUME")
	if out == "" {
		t.Skip("set ENTELOS_STORE_VOLUME to an output directory to run the store volume benchmark")
	}
	max := 20000
	if raw := os.Getenv("ENTELOS_STORE_VOLUME_MAX"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 100 {
			t.Fatal("ENTELOS_STORE_VOLUME_MAX must be an integer of 100 or more")
		}
		max = n
	}
	var maxBytes int64 = 4 << 30
	if raw := os.Getenv("ENTELOS_STORE_VOLUME_MAX_BYTES"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 1<<20 {
			t.Fatal("ENTELOS_STORE_VOLUME_MAX_BYTES must be an integer of 1048576 or more")
		}
		maxBytes = n
	}
	stopped := ""
	lastRetry := 0.0
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(out, "store.git")
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s exists; use a new output directory", path)
	}
	store, err := OpenGitStore(path)
	if err != nil {
		t.Fatal(err)
	}
	r := &volumeRun{t: t, path: path, store: store,
		runner: &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed"}}}
	root, err := store.Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	r.cursor = root.Cursor

	// The lifecycle path of runner_test.go, on the Git store.
	source := r.must("reviewer", "reviewer", "source", with(window, Object{"text": "visible rule", "reason": "first", "supersedes": Array{}}))["source_ref"]
	plan := r.must("reviewer", "reviewer", "plan", Object{"source_ref": source, "cases": Array{Object{"name": "one"}}, "origin": "independent", "supersedes": Array{}})["plan_ref"]
	snapshot := r.must("author", "author", "propose", with(window, Object{"base_snapshot": "", "source_ref": source, "bundle": bundle("pass"), "reason": "first"}))["snapshot_ref"]
	report := r.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})["report_ref"]
	decision := r.must("reviewer", "reviewer", "decide", Object{"report_ref": report, "reason": "volume"})["decision_ref"]
	r.must("reviewer", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{}})
	r.must("worker", "worker", "load", Object{"function": "record.visible", "date": "2026-11-15"})

	caller := r.host("caller", "caller", store)
	reviewer := r.host("reviewer", "reviewer", store)
	events := r.host("events", "event_reader", store)
	csv, err := os.Create(filepath.Join(out, "writes.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer csv.Close()
	fmt.Fprintln(csv, "receipt,write_ms")

	type checkpoint struct {
		Receipts      int     `json:"receipts"`
		StoreBytes    int64   `json:"store_bytes"`
		StateBytes    int     `json:"state_bytes"`
		WriteP50      float64 `json:"write_p50_ms"`
		WriteP95      float64 `json:"write_p95_ms"`
		WriteMax      float64 `json:"write_max_ms"`
		Inspect       float64 `json:"inspect_ms"`
		InspectCode   string  `json:"inspect_code"`
		Events        float64 `json:"decision_events_ms"`
		EventsCode    string  `json:"decision_events_code"`
		Retry         float64 `json:"retry_ms"`
		RetryCode     string  `json:"retry_code"`
		Resolve       float64 `json:"resolve_ms"`
		ResolveCode   string  `json:"resolve_code"`
		SegmentWrites int     `json:"segment_writes"`
	}
	var checkpoints []checkpoint
	marks := []int{}
	for _, n := range []int{100, 500, 1000, 2000, 5000, 10000, 20000, 50000, 100000} {
		if n < max {
			marks = append(marks, n)
		}
	}
	marks = append(marks, max)
	var segment []time.Duration
	for _, mark := range marks {
		if stopped != "" {
			break
		}
		segment = segment[:0]
		for r.written < mark {
			if r.written%100 == 0 && dirSize(t, path) > maxBytes {
				stopped = fmt.Sprintf("the store passed %d bytes at %d receipts", maxBytes, r.written)
				break
			}
			fields := invokeFields(r.written)
			if r.first == nil {
				r.first = Object{"op": "invoke", "expected_revision": r.cursor, "request_key": "v-" + strconv.Itoa(r.keys+1)}
				for k, v := range fields {
					r.first[k] = v
				}
			}
			start := time.Now()
			if _, err := r.write(caller, "invoke", fields); err != nil {
				t.Fatalf("receipt %d: %v", r.written, err)
			}
			d := time.Since(start)
			segment = append(segment, d)
			r.written++
			fmt.Fprintf(csv, "%d,%.3f\n", r.written, float64(d.Microseconds())/1000)
		}
		c := checkpoint{Receipts: r.written, StoreBytes: dirSize(t, path), SegmentWrites: len(segment)}
		c.WriteP50, c.WriteP95, c.WriteMax = quantiles(segment)
		view, err := store.Read(nil)
		if err != nil {
			t.Fatal(err)
		}
		c.StateBytes = len(Encode(view.State))
		c.Inspect, c.InspectCode = timed(func() error { _, err := reviewer.Handle(Object{"op": "inspect"}); return err })
		c.Events, c.EventsCode = timed(func() error {
			_, err := events.Handle(Object{"op": "decision_events", "limit": Int{Text: "256"}})
			return err
		})
		// A retry walks the history. Above one minute, later sizes skip it.
		c.Retry, c.RetryCode = 0, "skipped"
		if lastRetry < 60000 {
			c.Retry, c.RetryCode = timed(func() error {
				out, err := caller.Handle(r.first)
				if err == nil && out.(Object)["reused"] != true {
					return errors.New("retry was not reused")
				}
				return err
			})
			lastRetry = c.Retry
		}
		c.Resolve, c.ResolveCode = timed(func() error {
			_, err := r.host("reader", "reader", store).Handle(Object{"op": "resolve", "function": "record.visible", "date": "2026-11-15"})
			return err
		})
		checkpoints = append(checkpoints, c)
		t.Logf("%+v", c)
	}

	// Offered rates on one writer at the final size. A rate above the write
	// capacity gives a backlog, so the achieved rate is lower.
	type rate struct {
		Offered  int     `json:"offered_per_s"`
		Achieved float64 `json:"achieved_per_s"`
		Writes   int     `json:"writes"`
		P50      float64 `json:"write_p50_ms"`
		P95      float64 `json:"write_p95_ms"`
	}
	var rates []rate
	for _, offered := range []int{5, 10, 20, 50, 100} {
		window := 5 * time.Second
		interval := time.Second / time.Duration(offered)
		var lat []time.Duration
		start := time.Now()
		next := start
		for time.Since(start) < window {
			if wait := time.Until(next); wait > 0 {
				time.Sleep(wait)
			}
			next = next.Add(interval)
			s := time.Now()
			if _, err := r.write(caller, "invoke", invokeFields(r.written)); err != nil {
				t.Fatalf("rate %d: %v", offered, err)
			}
			lat = append(lat, time.Since(s))
			r.written++
		}
		x := rate{Offered: offered, Writes: len(lat), Achieved: float64(len(lat)) / time.Since(start).Seconds()}
		x.P50, x.P95, _ = quantiles(lat)
		rates = append(rates, x)
		t.Logf("%+v", x)
	}

	// Concurrent writers, each with its own store handle and Host. A writer
	// that gets stale_revision reads the root again and retries.
	type contention struct {
		Writers  int     `json:"writers"`
		Writes   int     `json:"writes"`
		Attempts int     `json:"attempts"`
		Stale    int     `json:"stale_revision"`
		PerS     float64 `json:"writes_per_s"`
	}
	var contentions []contention
	for _, writers := range []int{2, 4} {
		const each = 100
		var mu sync.Mutex
		c := contention{Writers: writers}
		var wg sync.WaitGroup
		start := time.Now()
		for w := 0; w < writers; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				handle, err := OpenGitStore(path)
				if err != nil {
					t.Error(err)
					return
				}
				h := r.host("caller", "caller", handle)
				for i := 0; i < each; i++ {
					for {
						view, err := handle.Read(nil)
						if err != nil {
							t.Error(err)
							return
						}
						q := Object{"op": "invoke", "expected_revision": view.Cursor, "request_key": fmt.Sprintf("c-%d-%d-%d", writers, w, i)}
						for k, v := range invokeFields(1000000 + writers*10000 + w*1000 + i) {
							q[k] = v
						}
						_, err = h.Handle(q)
						mu.Lock()
						c.Attempts++
						var fault *Fault
						stale := errors.As(err, &fault) && fault.Code == "stale_revision"
						if stale {
							c.Stale++
						} else if err == nil {
							c.Writes++
						}
						mu.Unlock()
						if err == nil {
							break
						}
						if !stale {
							t.Errorf("writer %d: %v", w, err)
							return
						}
					}
				}
			}(w)
		}
		wg.Wait()
		c.PerS = float64(c.Writes) / time.Since(start).Seconds()
		contentions = append(contentions, c)
		t.Logf("%+v", c)
	}

	raw, _ := json.Marshal(map[string]any{
		"go": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH, "cpus": runtime.NumCPU(),
		"checkpoints": checkpoints, "rates": rates, "contention": contentions,
		"final_store_bytes": dirSize(t, path), "stopped": stopped, "max_bytes": maxBytes,
	})
	if err := os.WriteFile(filepath.Join(out, "summary.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
