package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"pi-web/internal/chat"
)

type rpcResponse struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Command string          `json:"command"`
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

type rpcModel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

func readJSONLLines(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func writeRPCCommand(w io.Writer, cmd map[string]any) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func buildSwitchSessionCommand(id, sessionPath string) map[string]any {
	return map[string]any{"id": id, "type": "switch_session", "sessionPath": sessionPath}
}

func buildPromptCommand(id string, chat chat.Request, streaming bool) map[string]any {
	cmd := map[string]any{"id": id, "type": "prompt", "message": chat.Message}
	if len(chat.Images) > 0 {
		cmd["images"] = chat.Images
	}
	if streaming {
		cmd["streamingBehavior"] = "steer"
	}
	return cmd
}

func buildGetStateCommand(id string) map[string]any {
	return map[string]any{"id": id, "type": "get_state"}
}

func buildSetThinkingLevelCommand(id, level string) map[string]any {
	return map[string]any{"id": id, "type": "set_thinking_level", "level": level}
}
