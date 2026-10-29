// This client is test support. Credentials enter through stdin and never argv.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

type request struct {
	Kind    string          `json:"kind"`
	URL     string          `json:"url"`
	Origin  string          `json:"origin"`
	Cookie  string          `json:"cookie"`
	Action  string          `json:"action"`
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload"`
	Output  string          `json:"output"`
	Input   string          `json:"input"`
}

func main() {
	var input request
	decoder := json.NewDecoder(os.Stdin)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		os.Exit(2)
	}
	headers := http.Header{}
	headers.Set("Cookie", input.Cookie)
	headers.Set("Origin", input.Origin)
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	connection, _, err := dialer.Dial(input.URL, headers)
	if err != nil {
		os.Exit(3)
	}
	defer connection.Close()
	_ = connection.SetReadDeadline(time.Now().Add(70 * time.Second))
	if input.Kind == "terminal" {
		err = terminal(connection, input)
	} else {
		err = exchange(connection, input)
	}
	if err != nil {
		os.Exit(4)
	}
}

func exchange(connection *websocket.Conn, input request) error {
	if err := connection.WriteJSON(map[string]any{"type": "request", "action": input.Action, "id": input.ID, "payload": input.Payload}); err != nil {
		return err
	}
	for {
		_, data, err := connection.ReadMessage()
		if err != nil {
			return err
		}
		for _, frame := range bytes.Split(data, []byte{'\n'}) {
			var response map[string]any
			if json.Unmarshal(frame, &response) == nil && response["id"] == input.ID {
				return json.NewEncoder(os.Stdout).Encode(response)
			}
		}
	}
}

func terminal(connection *websocket.Conn, input request) error {
	output, err := os.OpenFile(input.Output, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer output.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		total := 0
		for {
			_, data, err := connection.ReadMessage()
			if err != nil {
				return
			}
			if total < 2<<20 {
				_, _ = output.Write(data)
				total += len(data)
			}
		}
	}()
	resize := func(cols, rows int) error {
		data, _ := json.Marshal(map[string]int{"cols": cols, "rows": rows})
		return connection.WriteMessage(websocket.BinaryMessage, append([]byte{1}, data...))
	}
	if err = resize(120, 40); err != nil {
		return err
	}
	time.Sleep(2 * time.Second)
	if err = connection.WriteMessage(websocket.BinaryMessage, []byte(input.Input+"\r")); err != nil {
		return err
	}
	time.Sleep(6 * time.Second)
	if err = resize(88, 28); err != nil {
		return err
	}
	select {
	case <-done:
		return errors.New("terminal closed before acceptance completed")
	case <-time.After(25 * time.Second):
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]bool{"input_sent": true, "resize_frames_sent": true})
}
