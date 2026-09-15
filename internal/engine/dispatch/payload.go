package dispatch

import (
	"encoding/json"
	"strings"
)

// payloadBody mirrors the JSON shape web/public/sw.js expects.
type payloadBody struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// DefaultPayload builds the PRD §4.2 default notification (no @payload
// override): title is the task's own title, body points at its file.
func DefaultPayload(task TaskContext) (string, error) {
	body := payloadBody{
		Title: task.Title,
		Body:  "Due from /" + strings.TrimPrefix(task.FilePath, "/"),
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
