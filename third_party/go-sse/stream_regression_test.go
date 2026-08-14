package sse

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestStreamRecvAfterEOF(t *testing.T) {
	t.Run("unterminated final event", func(t *testing.T) {
		stream := NewStream(io.NopCloser(strings.NewReader("data: hello\n")))

		event, err := stream.Recv()
		if err != nil {
			t.Fatalf("first Recv() error = %v", err)
		}
		if event.Data != "hello" {
			t.Fatalf("first Recv() data = %q, want %q", event.Data, "hello")
		}

		if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
			t.Fatalf("second Recv() error = %v, want EOF", err)
		}
		if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
			t.Fatalf("third Recv() error = %v, want EOF", err)
		}
	})

	t.Run("empty stream", func(t *testing.T) {
		stream := NewStream(io.NopCloser(strings.NewReader("")))

		if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
			t.Fatalf("first Recv() error = %v, want EOF", err)
		}
		if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
			t.Fatalf("second Recv() error = %v, want EOF", err)
		}
	})
}
