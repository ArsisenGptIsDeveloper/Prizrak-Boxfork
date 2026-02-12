package models

type ProcessBypass struct {
	Enable    bool     `json:"enable"`
	Processes []string `json:"processes"`
}
