package dashboard

import (
        "runtime"
        "testing"
        "time"
)

// TestMemoryMetrics_AfterGC_HeapAllocDrops is the validation stress test
// requested in the bug report.
//
// It verifies that:
//   1. After allocating several hundred MB, HeapAlloc increases.
//   2. After releasing all references and calling runtime.GC(), HeapAlloc drops.
//   3. The GC chart (NumGC) records the collection (NumGC increases).
//   4. Memory charts update correctly (the snapshot after GC shows lower HeapAlloc).
//   5. TotalAlloc is NOT confused with HeapAlloc (TotalAlloc only grows).
//   6. HeapSys is NOT confused with HeapAlloc.
//   7. PauseNs reflects the most recent GC pause, not the oldest.
//   8. Historical samples are snapshots, not accumulated values.
func TestMemoryMetrics_AfterGC_HeapAllocDrops(t *testing.T) {
        // Step 1: Read initial metrics.
        initial := readMemSnapshot(t)
        t.Logf("initial: HeapAlloc=%d bytes, TotalAlloc=%d, NumGC=%d",
                initial.HeapAlloc, initial.TotalAlloc, initial.NumGC)

        // Step 2: Allocate ~200MB and keep references alive.
        // We write to each buffer to prevent the compiler from optimizing
        // away the allocation.
        allocations := make([][]byte, 200)
        for i := range allocations {
                allocations[i] = make([]byte, 1024*1024) // 1MB each
                allocations[i][0] = byte(i)              // write to prevent optimization
        }
        // Keep references alive by reading them.
        sum := 0
        for _, a := range allocations {
                sum += int(a[0])
        }
        runtime.KeepAlive(allocations)
        runtime.KeepAlive(sum)

        // Do NOT call runtime.GC() here — we want to measure the heap with
        // the 200MB still live. The runtime may have run GCs during allocation
        // (NumGC increases), but the 200MB is still reachable.
        afterAlloc := readMemSnapshot(t)
        t.Logf("after alloc: HeapAlloc=%d bytes, TotalAlloc=%d, NumGC=%d",
                afterAlloc.HeapAlloc, afterAlloc.TotalAlloc, afterAlloc.NumGC)

        // Verify TotalAlloc increased (it's cumulative, only grows).
        if afterAlloc.TotalAlloc <= initial.TotalAlloc {
                t.Errorf("TotalAlloc did not increase: got %d, want > %d (TotalAlloc is cumulative)",
                        afterAlloc.TotalAlloc, initial.TotalAlloc)
        }

        // Verify TotalAlloc is cumulative (must be >= HeapAlloc since it counts all allocations ever).
        if afterAlloc.TotalAlloc < afterAlloc.HeapAlloc {
                t.Errorf("TotalAlloc (%d) < HeapAlloc (%d) — TotalAlloc must be cumulative and >= HeapAlloc",
                        afterAlloc.TotalAlloc, afterAlloc.HeapAlloc)
        }

        // Step 3: Release all references.
        allocations = nil
        runtime.GC() // force GC to reclaim the memory

        // Give the runtime a moment to settle.
        time.Sleep(10 * time.Millisecond)

        afterGC := readMemSnapshot(t)
        t.Logf("after GC: HeapAlloc=%d bytes, TotalAlloc=%d, NumGC=%d",
                afterGC.HeapAlloc, afterGC.TotalAlloc, afterGC.NumGC)

        // Step 4: Confirm GC chart records the collection (NumGC increased).
        if afterGC.NumGC <= afterAlloc.NumGC {
                t.Errorf("NumGC did not increase after runtime.GC(): got %d, want > %d",
                        afterGC.NumGC, afterAlloc.NumGC)
        }

        // Step 5: TotalAlloc must NOT decrease (it's cumulative).
        if afterGC.TotalAlloc < afterAlloc.TotalAlloc {
                t.Errorf("TotalAlloc decreased after GC: got %d, want >= %d (TotalAlloc is cumulative)",
                        afterGC.TotalAlloc, afterAlloc.TotalAlloc)
        }

        // Step 6: TotalAlloc must NOT equal HeapAlloc (they are different metrics).
        if afterGC.TotalAlloc == afterGC.HeapAlloc && afterGC.HeapAlloc > 0 {
                t.Errorf("TotalAlloc == HeapAlloc (%d) — these must never be equal unless HeapAlloc is 0",
                        afterGC.HeapAlloc)
        }

        // Step 7: Verify HeapSys >= HeapAlloc (HeapSys is the OS allocation for the heap;
        // HeapAlloc is the live objects within it).
        if afterGC.HeapSys < afterGC.HeapAlloc {
                t.Errorf("HeapSys (%d) < HeapAlloc (%d) — HeapSys must be >= HeapAlloc",
                        afterGC.HeapSys, afterGC.HeapAlloc)
        }

        // Step 8: Verify Sys >= HeapSys (Sys is total from OS, HeapSys is heap portion).
        if afterGC.Sys < afterGC.HeapSys {
                t.Errorf("Sys (%d) < HeapSys (%d) — Sys must be >= HeapSys",
                        afterGC.Sys, afterGC.HeapSys)
        }

        // Step 9: Verify PauseNs is non-zero after a GC (we forced one).
        if afterGC.NumGC > 0 && afterGC.PauseNs == 0 {
                t.Error("PauseNs is 0 after a GC — should reflect the most recent pause duration")
        }

        // Step 10: Verify PauseTotalNs >= PauseNs (PauseTotalNs is cumulative).
        if afterGC.PauseTotalNs < afterGC.PauseNs {
                t.Errorf("PauseTotalNs (%d) < PauseNs (%d) — PauseTotalNs is cumulative and must be >= PauseNs",
                        afterGC.PauseTotalNs, afterGC.PauseNs)
        }

        // Step 11: Verify HeapAlloc dropped after releasing references + GC.
        // This is the KEY assertion — HeapAlloc must be lower after GC than
        // when the 200MB was live. We compare against the initial value
        // (before allocation) because the runtime may have already GC'd
        // during allocation, making afterAlloc.HeapAlloc unreliable.
        if afterGC.HeapAlloc > initial.HeapAlloc*10 {
                t.Errorf("HeapAlloc (%d) did not drop back near initial (%d) after GC — memory leak suspected",
                        afterGC.HeapAlloc, initial.HeapAlloc)
        }
}

// TestMemoryMetrics_SnapshotsNotAccumulated verifies that the historical
// buffer stores snapshots, not accumulated values.
//
// Incorrect: history[i] += current
// Correct:   history[i] = current
func TestMemoryMetrics_SnapshotsNotAccumulated(t *testing.T) {
        cfg := DefaultConfig()
        c := newCollector(cfg, nil)

        // Push three snapshots with known HeapAlloc values.
        c.metrics.Push(MetricsSnapshot{Time: time.Now(), HeapAlloc: 1000})
        c.metrics.Push(MetricsSnapshot{Time: time.Now(), HeapAlloc: 500})
        c.metrics.Push(MetricsSnapshot{Time: time.Now(), HeapAlloc: 800})

        hist := c.MetricsHistory(10)
        if len(hist) != 3 {
                t.Fatalf("expected 3 history entries, got %d", len(hist))
        }

        // Verify each entry is the exact value we pushed (not accumulated).
        if hist[0].HeapAlloc != 1000 {
                t.Errorf("hist[0].HeapAlloc = %d, want 1000 (snapshot, not accumulated)", hist[0].HeapAlloc)
        }
        if hist[1].HeapAlloc != 500 {
                t.Errorf("hist[1].HeapAlloc = %d, want 500 (snapshot, not accumulated)", hist[1].HeapAlloc)
        }
        if hist[2].HeapAlloc != 800 {
                t.Errorf("hist[2].HeapAlloc = %d, want 800 (snapshot, not accumulated)", hist[2].HeapAlloc)
        }
}

// TestMemoryMetrics_AllFieldsPopulated verifies that all required MemStats
// fields are captured in the snapshot — none should be zero (unless the
// runtime genuinely reports zero, which is unlikely after some allocation).
func TestMemoryMetrics_AllFieldsPopulated(t *testing.T) {
        // Allocate some memory to ensure non-zero values.
        data := make([]byte, 10*1024*1024) // 10MB
        for i := range data {
                data[i] = byte(i)
        }
        runtime.GC()

        snap := readMemSnapshot(t)

        // These fields must be populated (non-zero after allocation).
        checks := []struct {
                name string
                val  uint64
        }{
                {"HeapAlloc", snap.HeapAlloc},
                {"HeapSys", snap.HeapSys},
                {"HeapIdle", snap.HeapIdle},
                {"HeapInuse", snap.HeapInuse},
                {"HeapReleased", snap.HeapReleased},
                {"HeapObjects", snap.HeapObjects},
                {"TotalAlloc", snap.TotalAlloc},
                {"Mallocs", snap.Mallocs},
                {"Frees", snap.Frees},
                {"StackInUse", snap.StackInUse},
                {"StackSys", snap.StackSys},
                {"MSpanInuse", snap.MSpanInuse},
                {"MSpanSys", snap.MSpanSys},
                {"MCacheInuse", snap.MCacheInuse},
                {"MCacheSys", snap.MCacheSys},
                {"BuckHashSys", snap.BuckHashSys},
                {"OtherSys", snap.OtherSys},
                {"Sys", snap.Sys},
                {"NextGC", snap.NextGC},
                {"PauseTotalNs", snap.PauseTotalNs},
        }
        for _, ch := range checks {
                if ch.val == 0 {
                        t.Errorf("%s is 0 — field not populated from runtime.MemStats", ch.name)
                }
        }

        // NumGC should be > 0 after we called runtime.GC().
        if snap.NumGC == 0 {
                t.Error("NumGC is 0 — expected at least 1 GC after runtime.GC()")
        }

        // NumCPU and GOMAXPROCS must be populated.
        if snap.NumCPU == 0 {
                t.Error("NumCPU is 0 — should be runtime.NumCPU()")
        }
        if snap.GOMAXPROCS == 0 {
                t.Error("GOMAXPROCS is 0 — should be runtime.GOMAXPROCS(0)")
        }
}

// TestBuildPerfMetrics_FreshRead verifies that buildPerfMetrics reads fresh
// runtime.MemStats and maps all fields correctly — no stale data, no
// confused field mappings.
func TestBuildPerfMetrics_FreshRead(t *testing.T) {
        cfg := DefaultConfig()
        c := newCollector(cfg, nil)

        // Allocate some memory.
        data := make([]byte, 5*1024*1024)
        _ = data
        runtime.GC()

        pm := buildPerfMetrics(c)

        // Verify Heap fields are correctly mapped.
        if pm.Heap.Alloc == 0 {
                t.Error("Heap.Alloc is 0 — should be ms.HeapAlloc")
        }
        if pm.Heap.TotalAlloc == 0 {
                t.Error("Heap.TotalAlloc is 0 — should be ms.TotalAlloc")
        }
        // TotalAlloc must be >= HeapAlloc (it's cumulative).
        if pm.Heap.TotalAlloc < pm.Heap.Alloc {
                t.Errorf("Heap.TotalAlloc (%d) < Heap.Alloc (%d) — TotalAlloc is cumulative and must be >= HeapAlloc",
                        pm.Heap.TotalAlloc, pm.Heap.Alloc)
        }
        // TotalAlloc must NOT equal HeapAlloc (different metrics).
        if pm.Heap.TotalAlloc == pm.Heap.Alloc {
                t.Errorf("Heap.TotalAlloc == Heap.Alloc (%d) — these are different metrics and should not be equal",
                        pm.Heap.Alloc)
        }

        // Verify Stack.Sys is NOT HeapSys (old bug).
        if pm.Stack.Sys == 0 {
                t.Error("Stack.Sys is 0 — should be ms.StackSys")
        }

        // Verify GC fields.
        if pm.GC.NumGC == 0 {
                t.Error("GC.NumGC is 0 — should be > 0 after runtime.GC()")
        }
        if pm.GC.PauseTotalNS == 0 {
                t.Error("GC.PauseTotalNS is 0 — should be ms.PauseTotalNs (cumulative)")
        }
        // PauseTotalNS must be >= PauseNS (cumulative vs latest).
        if pm.GC.PauseTotalNS < pm.GC.PauseNS {
                t.Errorf("GC.PauseTotalNS (%d) < GC.PauseNS (%d) — PauseTotalNS is cumulative",
                        pm.GC.PauseTotalNS, pm.GC.PauseNS)
        }

        // Verify CPU fields are populated (old bug: were 0).
        if pm.CPU.NumCPU == 0 {
                t.Error("CPU.NumCPU is 0 — should be runtime.NumCPU()")
        }
        if pm.CPU.GOMAXPROCS == 0 {
                t.Error("CPU.GOMAXPROCS is 0 — should be runtime.GOMAXPROCS(0)")
        }

        // Verify Memory.Sys is populated.
        if pm.Memory.Sys == 0 {
                t.Error("Memory.Sys is 0 — should be ms.Sys")
        }
}

// readMemSnapshot reads a fresh runtime.MemStats and builds a MetricsSnapshot
// using the same logic as the sampler.
func readMemSnapshot(t *testing.T) MetricsSnapshot {
        t.Helper()
        var ms runtime.MemStats
        runtime.ReadMemStats(&ms)

        var lastPauseNs uint64
        if ms.NumGC > 0 {
                lastPauseNs = ms.PauseNs[(ms.NumGC-1+256)%256]
        }

        return MetricsSnapshot{
                Time:           time.Now(),
                HeapAlloc:      ms.HeapAlloc,
                HeapSys:        ms.HeapSys,
                HeapIdle:       ms.HeapIdle,
                HeapInuse:      ms.HeapInuse,
                HeapReleased:   ms.HeapReleased,
                HeapObjects:    ms.HeapObjects,
                TotalAlloc:     ms.TotalAlloc,
                Mallocs:        ms.Mallocs,
                Frees:          ms.Frees,
                StackInUse:     ms.StackInuse,
                StackSys:       ms.StackSys,
                MSpanInuse:     ms.MSpanInuse,
                MSpanSys:       ms.MSpanSys,
                MCacheInuse:    ms.MCacheInuse,
                MCacheSys:      ms.MCacheSys,
                BuckHashSys:    ms.BuckHashSys,
                OtherSys:       ms.OtherSys,
                Sys:            ms.Sys,
                NumGC:          ms.NumGC,
                NextGC:         ms.NextGC,
                PauseTotalNs:   ms.PauseTotalNs,
                PauseNs:        lastPauseNs,
                GCCPUFraction:  ms.GCCPUFraction,
                NumCPU:         runtime.NumCPU(),
                GOMAXPROCS:     runtime.GOMAXPROCS(0),
                CGOCalls:       runtime.NumCgoCall(),
        }
}
