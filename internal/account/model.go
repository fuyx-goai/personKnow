package account

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSessionInvalid = errors.New("登录会话无效")
	ErrTicketInvalid  = errors.New("扫码登录票据无效")
	ErrWeChatLogin    = errors.New("微信登录失败")
	ErrUserDisabled   = errors.New("账号已停用")
)

type UserStatus string

const (
	UserActive   UserStatus = "active"
	UserDisabled UserStatus = "disabled"
)

type ClientType string

const (
	ClientMiniProgram ClientType = "miniprogram"
	ClientWeb         ClientType = "web"
)

type TicketStatus string

const (
	TicketPending   TicketStatus = "pending"
	TicketConfirmed TicketStatus = "confirmed"
	TicketConsumed  TicketStatus = "consumed"
	TicketExpired   TicketStatus = "expired"
)

type User struct {
	ID        uuid.UUID  `json:"id"`
	Nickname  string     `json:"nickname"`
	AvatarURL string     `json:"avatar_url,omitempty"`
	Status    UserStatus `json:"status"`
}

type Profile struct {
	Nickname  string
	AvatarURL string
}

type WeChatIdentity struct {
	AppID   string
	OpenID  string
	UnionID string
}

type Session struct {
	ID               uuid.UUID  `json:"id"`
	UserID           uuid.UUID  `json:"-"`
	RefreshTokenHash string     `json:"-"`
	ClientType       ClientType `json:"client_type"`
	DeviceLabel      string     `json:"device_label,omitempty"`
	ExpiresAt        time.Time  `json:"expires_at"`
	RevokedAt        *time.Time `json:"-"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type WebLoginTicket struct {
	ID              uuid.UUID
	SecretHash      string
	Status          TicketStatus
	ConfirmedUserID *uuid.UUID
	ExpiresAt       time.Time
}

type LoginCommand struct {
	Code        string
	Profile     Profile
	ClientType  ClientType
	DeviceLabel string
}

type Identity struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
}

type AuthResult struct {
	TicketStatus TicketStatus `json:"status,omitempty"`
	User         User         `json:"user,omitempty"`
	AccessToken  string       `json:"access_token,omitempty"`
	RefreshToken string       `json:"refresh_token,omitempty"`
	ExpiresIn    int64        `json:"expires_in,omitempty"`
}

type TicketResult struct {
	ID        uuid.UUID `json:"id"`
	Secret    string    `json:"secret"`
	QRPayload string    `json:"qr_payload"`
	ExpiresAt time.Time `json:"expires_at"`
}
