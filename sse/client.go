package sse

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultReconnectionTime = time.Millisecond * 2500
)

type Event struct {
	LastEventId string
	EventType   string
	Data        string
}

type EventSource struct {
	lastEventId      string
	reconnectionTime time.Duration

	dataBuf        string
	eventTypeBuf   string
	lastEventIdBuf string

	HttpClient *http.Client
	Handle     func(Event, error)
}

func (es *EventSource) Connect(req *http.Request) error {
	if es.reconnectionTime == 0 {
		es.reconnectionTime = defaultReconnectionTime
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	for {
		select {
		case <-req.Context().Done():
			return req.Context().Err()
		default:
		}

		if es.lastEventId != "" {
			req.Header.Set("Last-Event-ID", es.lastEventId)
		}

		resp, err := es.HttpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return fmt.Errorf("failed to connect: response status %d", resp.StatusCode)
		}
		if resp.Header.Get("Content-Type") != "text/event-stream" {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return fmt.Errorf("failed to connect: invalid response content type %q", resp.Header.Get("Content-Type"))
		}

		readErr := func() error {
			defer resp.Body.Close()
			streamErr := es.readSourceStream(resp.Body)
			if streamErr != nil {
				if streamErr == io.EOF {
					return nil // Clean disconnection
				}
				if es.Handle != nil {
					es.Handle(Event{}, streamErr)
				}
				return streamErr
			}
			return nil
		}()

		if readErr != nil {
			select {
			case <-req.Context().Done():
				return req.Context().Err()
			case <-time.After(es.reconnectionTime):
			}
		}
	}
}

func (es *EventSource) readSourceStream(r io.Reader) error {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation

	// Ignore initial Byte Order Mark (BOM)
	br := bufio.NewReader(r)
	ch, _, err := br.ReadRune()
	if err != nil {
		return err
	}
	if ch != '\uFEFF' {
		br.UnreadRune()
	}

	scanner := bufio.NewScanner(br)
	for scanner.Scan() {
		ln := scanner.Text()
		// if the line is empty, dispatch the event
		if ln == "" {
			es.dispatch()
			continue
		}
		// if the line starts with a ":", ignore the line
		if strings.HasPrefix(ln, ":") {
			continue
		}
		// if the line contains a ":":
		//   + field is the characters before the first ":"
		//   + value is the characters after the first ":"
		//   + process the field and value
		if strings.Contains(ln, ":") {
			parts := strings.SplitN(ln, ":", 2)
			if len(parts) != 2 {
				// this should never occur
				return fmt.Errorf("failed to parse line %q: got %d parts, want 2", ln, len(parts))
			}
			field, value := parts[0], parts[1]
			value = strings.TrimPrefix(value, " ")
			es.processField(field, value)
			continue
		}
		// otherwise, process the whole line as the field and an empty string
		// value
		es.processField(ln, "")
	}

	return scanner.Err()
}

func (es *EventSource) processField(field, value string) error {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation

	switch field {
	case "event":
		// set the event type buffer to the field value
		es.eventTypeBuf = value
	case "data":
		// append the field value to the data buffer, followed by an "\n"
		es.dataBuf += value + "\n"
	case "id":
		// if the field value does not contain "\0", set the last event id
		// buffer to the field value. otherwise, ignore the field
		if !strings.ContainsRune(value, '\x00') {
			es.lastEventIdBuf = value
		}
	case "retry":
		// if the field value consists of only ASCII digist, interpret it as a
		// base 10 integer, and set the stream's reconnection time. otherwise,
		// ignore the field
		if allASCIIDigits(value) {
			ms, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("failed to parse retry field value %q: %w", value, err)
			}
			es.reconnectionTime = time.Millisecond * time.Duration(ms)
		}
	default:
		// ignore the field
	}
	return nil
}

func (es *EventSource) dispatch() {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#dispatchMessage

	// 1. set the last event ID to the value of the last event ID buffer
	es.lastEventId = es.lastEventIdBuf

	// 2. if the data buffer is empty, reset the event type buffer and return
	if es.dataBuf == "" {
		es.eventTypeBuf = ""
		return
	}

	// 3. if the data buffer's last char is "\n", remove it
	es.dataBuf = strings.TrimSuffix(es.dataBuf, "\n")

	// 4. create an event ...
	var e Event

	// 5. init the type attribute to "message", the data attribute, and the last
	//    event ID attribute
	e.EventType = "message"
	e.Data = es.dataBuf
	e.LastEventId = es.lastEventId

	// 6. if the event type buffer is non-empty, set the event type attribute
	if es.eventTypeBuf != "" {
		e.EventType = es.eventTypeBuf
	}

	// 7. reset the data buffer and the event type buffer
	es.dataBuf = ""
	es.eventTypeBuf = ""

	// 8. queue the event
	if es.Handle != nil {
		es.Handle(e, nil)
	}
}

func allASCIIDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
