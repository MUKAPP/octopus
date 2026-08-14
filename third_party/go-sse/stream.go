package sse

import (
	"io"
	"strconv"
	"strings"

	"github.com/tmaxmax/go-sse/internal/parser"
)

// StreamConfig is used to configure how Stream behaves.
type StreamConfig struct {
	// MaxEventSize is the maximum expected length of the byte sequence
	// representing a single event. Parsing events longer than that
	// will result in an error.
	//
	// By default this limit is 64KB. You don't need to set this if it
	// is enough for your needs (e.g. the events you receive don't contain
	// larger amounts of data).
	MaxEventSize int
}

// Stream provides a convenient interface for reading SSE events one by one
// from an io.ReadCloser. It maintains state between calls to Recv() and
// handles the SSE protocol parsing internally.
type Stream struct {
	reader      io.ReadCloser
	parser      *parser.Parser
	lastEventID string
	closed      bool

	// Internal state for parsing
	typ   string
	sb    strings.Builder
	dirty bool
}

// Recv reads and returns the next event from the stream.
// It returns io.EOF when the stream is closed or reaches end of input.
// The Event.LastEventID field is maintained across calls, following
// the SSE specification behavior.
func (s *Stream) Recv() (Event, error) {
	if s.closed {
		return Event{}, io.EOF
	}

	for {
		f := parser.Field{}
		if !s.parser.Next(&f) {
			err := s.parser.Err()
			isEOF := err == io.EOF
			isUnexpectedEOF := err == parser.ErrUnexpectedEOF

			// If we have a dirty event at EOF or unexpected EOF, yield it
			if s.dirty && (isEOF || isUnexpectedEOF) {
				event := s.buildEvent()
				s.resetState()
				return event, nil
			}

			if err != nil && !isEOF && !isUnexpectedEOF {
				return Event{}, err
			}
			return Event{}, io.EOF
		}

		switch f.Name {
		case parser.FieldNameData:
			s.sb.WriteString(f.Value)
			s.sb.WriteByte('\n')
			s.dirty = true
		case parser.FieldNameEvent:
			s.typ = f.Value
			s.dirty = true
		case parser.FieldNameID:
			// empty IDs are valid, only IDs that contain the null byte must be ignored:
			// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
			if strings.IndexByte(f.Value, 0) == -1 {
				s.lastEventID = f.Value
				s.dirty = true
			}
		case parser.FieldNameRetry:
			// Parse retry field but don't handle it in Stream (similar to Read function)
			if _, err := strconv.ParseInt(f.Value, 10, 64); err == nil {
				s.dirty = true
			}
		default:
			// End of event - yield if we have data
			if s.dirty {
				event := s.buildEvent()
				s.resetState()
				return event, nil
			}
		}
	}
}

// buildEvent constructs an Event from the current state
func (s *Stream) buildEvent() Event {
	data := s.sb.String()
	if data != "" {
		data = data[:len(data)-1] // Remove trailing newline
	}
	return Event{
		LastEventID: s.lastEventID,
		Type:        s.typ,
		Data:        data,
	}
}

// resetState resets the internal parsing state for the next event
func (s *Stream) resetState() {
	s.sb.Reset()
	s.typ = ""
	s.dirty = false
}

// Close closes the underlying reader and marks the stream as closed.
func (s *Stream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.reader.Close()
}

// NewStream creates a new Stream from an io.ReadCloser with optional configuration.
func NewStream(r io.ReadCloser) *Stream {
	return NewStreamWithConfig(r, nil)
}

// NewStreamWithConfig creates a new Stream from an io.ReadCloser with the given configuration.
func NewStreamWithConfig(r io.ReadCloser, cfg *StreamConfig) *Stream {
	p := parser.New(r)
	if cfg != nil && cfg.MaxEventSize > 0 {
		p.Buffer(nil, cfg.MaxEventSize)
	}

	return &Stream{
		reader: r,
		parser: p,
	}
}
