package ai

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// SSEWriter writes Server-Sent Events to an HTTP response. It bypasses the
// project's JSON envelope on purpose: streaming endpoints speak the SSE
// protocol, not the {data,error} envelope.
type SSEWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

// NewSSEWriter sets the streaming response headers and returns the writer.
// ok is false when the ResponseWriter cannot flush (should not happen with
// the project's middleware, which forwards Flush).
func NewSSEWriter(w http.ResponseWriter) (*SSEWriter, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	// Disable proxy buffering (nginx) so events reach the client immediately.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	return &SSEWriter{w: w, f: f}, true
}

// WriteEvent sends one named event with a JSON payload.
func (s *SSEWriter) WriteEvent(event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal sse event %q: %w", event, err)
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return err
	}
	s.f.Flush()
	return nil
}

// Comment sends an SSE comment (": text"), used as a keep-alive ping.
func (s *SSEWriter) Comment(text string) {
	fmt.Fprintf(s.w, ": %s\n\n", text)
	s.f.Flush()
}
