package sse

import (
	"bufio"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

type Event struct {
	ID   string
	Type string
	Data string
}

type Client struct {
	HTTPClient *http.Client
}

type ConnectConfig struct {
	Types []string
}

func (c *Client) Connect(req *http.Request, msgs chan<- Event, cfg ConnectConfig) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if msgs == nil {
		return fmt.Errorf("messages channel is nil")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)

	var event Event
	var data []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if line == ":" {
			continue
		}

		if line == "" {
			// event is done
			if len(data) > 0 {
				event.Data = strings.Join(data, "\n")
				data = nil
			}
			if len(cfg.Types) == 0 || slices.Contains(cfg.Types, event.Type) {
				msgs <- event
			}
			event = Event{}
			continue
		}

		split := strings.SplitN(line, ":", 2)
		if len(split) != 2 {
			continue
		}

		key, value := split[0], split[1]
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}
		switch key {
		case "id":
			event.ID = value
		case "event":
			event.Type = value
		case "data":
			data = append(data, value)
		}
	}
}
