package main

import (
	"bytes"
	"io"
)

// RepeatingBuffer is a string buffer that repeats the same string after
// reaching EOF. Useful to send the same input to a reader multiple times, for
// example, to retry sending something to a processes' stdin a few retries.
type RepeatingBuffer struct {
	s string
	b *bytes.Buffer
}

func NewRepeatingBuffer(s string) *RepeatingBuffer {
	return &RepeatingBuffer{
		b: bytes.NewBufferString(s),
		s: s,
	}
}

func (r *RepeatingBuffer) Read(p []byte) (n int, err error) {
	n, err = r.b.Read(p)
	if err == io.EOF {
		r.b = bytes.NewBufferString(r.s)
	}
	return n, err
}
