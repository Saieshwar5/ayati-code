package shell

import (
	"fmt"
	"sync"
)

type boundedBuffer struct {
	mu      sync.Mutex
	limit   int
	headCap int
	tailCap int
	head    []byte
	tail    []byte
	total   int64
}

func newBoundedBuffer(limit int) *boundedBuffer {
	if limit <= 0 {
		limit = 1
	}
	headCap := limit / 2
	return &boundedBuffer{limit: limit, headCap: headCap, tailCap: limit - headCap}
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	written := len(value)
	b.total += int64(written)
	if remaining := b.headCap - len(b.head); remaining > 0 {
		take := remaining
		if take > len(value) {
			take = len(value)
		}
		b.head = append(b.head, value[:take]...)
		value = value[take:]
	}
	if len(value) == 0 || b.tailCap == 0 {
		return written, nil
	}
	if len(value) >= b.tailCap {
		b.tail = append(b.tail[:0], value[len(value)-b.tailCap:]...)
		return written, nil
	}
	if overflow := len(b.tail) + len(value) - b.tailCap; overflow > 0 {
		copy(b.tail, b.tail[overflow:])
		b.tail = b.tail[:len(b.tail)-overflow]
	}
	b.tail = append(b.tail, value...)
	return written, nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.total <= int64(b.limit) {
		combined := make([]byte, 0, len(b.head)+len(b.tail))
		combined = append(combined, b.head...)
		combined = append(combined, b.tail...)
		return string(combined)
	}
	omitted := b.total - int64(len(b.head)+len(b.tail))
	return string(b.head) + fmt.Sprintf("\n... %d bytes omitted ...\n", omitted) + string(b.tail)
}

func (b *boundedBuffer) Total() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.total
}

func (b *boundedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.total > int64(b.limit)
}
