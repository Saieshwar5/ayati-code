package exec

// boundedBuffer keeps at most limit bytes and records whether input was
// truncated, mirroring the previous sandbox output bounds.
type boundedBuffer struct {
	data      []byte
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	available := b.limit - len(b.data)
	if available > len(value) {
		available = len(value)
	}
	if available > 0 {
		b.data = append(b.data, value[:available]...)
	}
	if available < len(value) {
		b.truncated = true
	}
	return len(value), nil
}

func (b *boundedBuffer) String() string { return string(b.data) }
