package lifecycle

import (
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestStreamCoalescerFirstChunkIsImmediateAndAppendsAreWindowed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var got []coalescedStreamChunk
		var gotMu sync.Mutex
		coalescer := newStreamCoalescer(100*time.Millisecond, func(chunk coalescedStreamChunk) {
			gotMu.Lock()
			defer gotMu.Unlock()
			got = append(got, chunk)
		})

		coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "a"})
		gotMu.Lock()
		if len(got) != 1 || got[0].content != "a" {
			gotMu.Unlock()
			t.Fatalf("first chunk = %#v, want immediate publication", got)
		}
		gotMu.Unlock()

		coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "b", isAppend: true})
		coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "c", isAppend: true})
		gotMu.Lock()
		if len(got) != 1 {
			gotMu.Unlock()
			t.Fatalf("append chunks published before window: %d", len(got))
		}
		gotMu.Unlock()

		time.Sleep(99 * time.Millisecond)
		synctest.Wait()
		gotMu.Lock()
		if len(got) != 1 {
			gotMu.Unlock()
			t.Fatalf("append flushed before window: %d", len(got))
		}
		gotMu.Unlock()
		time.Sleep(1 * time.Millisecond)
		synctest.Wait()
		gotMu.Lock()
		defer gotMu.Unlock()
		if len(got) != 2 || got[1].content != "bc" || !got[1].isAppend {
			t.Fatalf("windowed append = %#v, want one bc append", got)
		}
	})
}

func TestStreamCoalescerPreservesLargeBurstExactly(t *testing.T) {
	var got []coalescedStreamChunk
	coalescer := newStreamCoalescer(time.Hour, func(chunk coalescedStreamChunk) {
		got = append(got, chunk)
	})

	const chunkCount = 30_000
	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "a"})
	for i := 1; i < chunkCount; i++ {
		coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "b", isAppend: true})
	}
	coalescer.flush()

	if len(got) != 2 {
		t.Fatalf("publication count = %d, want 2", len(got))
	}
	var content strings.Builder
	for _, chunk := range got {
		content.WriteString(chunk.content)
	}
	if content.String() != "a"+strings.Repeat("b", chunkCount-1) {
		t.Fatalf("burst content was not preserved exactly")
	}
}

func TestStreamCoalescerFlushesBeforeInterleavedRecord(t *testing.T) {
	var got []coalescedStreamChunk
	coalescer := newStreamCoalescer(time.Hour, func(chunk coalescedStreamChunk) {
		got = append(got, chunk)
	})

	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "a", content: "a"})
	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "a", content: "1", isAppend: true})
	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "b", content: "b"})
	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "b", content: "2", isAppend: true})
	coalescer.flush()

	if len(got) != 4 {
		t.Fatalf("publication count = %d, want 4 ordered segments", len(got))
	}
	want := []string{"a", "1", "b", "2"}
	for index, chunk := range got {
		if chunk.content != want[index] {
			t.Fatalf("chunk %d = %#v, want content %q", index, chunk, want[index])
		}
	}
}

func TestStreamCoalescerBoundaryMakesNextAppendImmediate(t *testing.T) {
	var got []coalescedStreamChunk
	coalescer := newStreamCoalescer(time.Hour, func(chunk coalescedStreamChunk) {
		got = append(got, chunk)
	})

	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "a"})
	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "b", isAppend: true})
	coalescer.flushBoundary()
	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "c", isAppend: true})

	if len(got) != 3 || got[1].content != "b" || got[2].content != "c" {
		t.Fatalf("boundary output = %#v, want separate b and immediate c segments", got)
	}
}

func TestStreamCoalescerCloseFlushesAndRejectsFutureChunks(t *testing.T) {
	var got []coalescedStreamChunk
	coalescer := newStreamCoalescer(time.Hour, func(chunk coalescedStreamChunk) {
		got = append(got, chunk)
	})

	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "a"})
	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "b", isAppend: true})
	coalescer.close()
	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "c", isAppend: true})
	coalescer.close()

	if len(got) != 2 || got[1].content != "b" {
		t.Fatalf("close output = %#v, want first plus final b segment", got)
	}
}

func TestStreamCoalescerStatsCountReceivedMergedAndFlushedSegments(t *testing.T) {
	coalescer := newStreamCoalescer(time.Hour, func(coalescedStreamChunk) {})
	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "a"})
	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "b", isAppend: true})
	coalescer.add(coalescedStreamChunk{eventType: "thinking_streaming", messageID: "m1", content: "c", isAppend: true})
	coalescer.flush()

	if got := coalescer.stats(); got.received != 3 || got.coalesced != 2 || got.flushed != 1 {
		t.Fatalf("stats = %+v, want received=3 coalesced=2 flushed=1", got)
	}
}
