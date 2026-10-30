package rpc

import "sync"

// stderr can still be written while an RPC EOF or retirement is being handled.
// Keep a bounded, synchronized tail rather than sharing a strings.Builder.
type workerStderr struct {
	mu   sync.Mutex
	tail []byte
}

func (b *workerStderr) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	const limit = 4096
	n := len(p)
	if n >= limit {
		b.tail = append(b.tail[:0], p[n-limit:]...)
	} else {
		if len(b.tail)+n > limit {
			b.tail = b.tail[len(b.tail)+n-limit:]
		}
		b.tail = append(b.tail, p...)
	}
	return n, nil
}

func (b *workerStderr) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.tail)
}
