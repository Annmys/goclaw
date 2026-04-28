package channels

import "encoding/json"

type writerLabelMeta struct {
	DisplayName string `json:"displayName"`
	Username    string `json:"username"`
}

func WriterLabel(raw json.RawMessage, userID string) string {
	var meta writerLabelMeta
	_ = json.Unmarshal(raw, &meta)
	if meta.Username != "" {
		return "@" + meta.Username
	}
	if meta.DisplayName != "" {
		return meta.DisplayName
	}
	return "User " + userID
}
