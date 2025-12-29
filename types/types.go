package types

import "time"

type UserData struct {
	UserID          string    `json:"user_id"`
	LastMessages    []string  `json:"last_messages"`
	GifURL          string    `json:"gif_url"`
	OffensiveCount  int       `json:"offensive_count"`
	WeeklyOffensive int       `json:"weekly_offensive"`
	LastWeekReset   time.Time `json:"last_week_reset"`
}

type Database struct {
	Users map[string]*UserData `json:"users"`
}

type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type OllamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}
