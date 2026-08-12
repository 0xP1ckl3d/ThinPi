package app

import "encoding/json"

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	IsAdmin     bool   `json:"is_admin"`
	Enabled     bool   `json:"enabled"`
}

type Connection struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Protocol       string          `json:"protocol"`
	Host           string          `json:"host,omitempty"`
	Port           int             `json:"port,omitempty"`
	Enabled        bool            `json:"enabled"`
	Icon           string          `json:"icon,omitempty"`
	SortOrder      int             `json:"sort_order"`
	ProtocolConfig json.RawMessage `json:"protocol_config,omitempty"`
	CredentialID   *int64          `json:"credential_id,omitempty"`
}

type Device struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Identifier string `json:"device_identifier"`
	Enabled    bool   `json:"enabled"`
}

type Session struct {
	ID       int64
	User     User
	CSRFHash string
}

type LaunchManifest struct {
	TicketID          int64           `json:"ticket_id"`
	ConnectionID      int64           `json:"connection_id"`
	Name              string          `json:"name"`
	Protocol          string          `json:"protocol"`
	Host              string          `json:"host"`
	Port              int             `json:"port"`
	Username          string          `json:"username,omitempty"`
	Password          string          `json:"password,omitempty"`
	Config            json.RawMessage `json:"config"`
	MaxSessionSeconds int             `json:"max_session_seconds,omitempty"`
}

type AccessPolicy struct {
	UserID            int64  `json:"user_id"`
	Timezone          string `json:"timezone"`
	AllowedDaysMask   int    `json:"allowed_days_mask"`
	AccessStartMinute int    `json:"access_start_minute"`
	AccessEndMinute   int    `json:"access_end_minute"`
	DailyLimitMinutes int    `json:"daily_limit_minutes"`
	MaxSessionMinutes int    `json:"max_session_minutes"`
}

type PolicyStatus struct {
	Allowed             bool   `json:"allowed"`
	Reason              string `json:"reason,omitempty"`
	Timezone            string `json:"timezone"`
	DailyUsedMinutes    int    `json:"daily_used_minutes"`
	DailyLimitMinutes   int    `json:"daily_limit_minutes"`
	MaxSessionMinutes   int    `json:"max_session_minutes"`
	EffectiveMaxSeconds int    `json:"effective_max_seconds"`
}
