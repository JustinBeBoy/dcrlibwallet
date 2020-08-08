package dcrlibwallet

import (
	"fmt"
	"net/http"
	"time"

	"github.com/asdine/storm"
)

const (
	politeiaConfigDbName = "politeia_config.db"
)

var (
	configDB *storm.DB
)

type PoliteiaConfig struct {
	ID                 int `storm:"id"`
	CsrfToken          string
	Cookies            []*http.Cookie
	SessionExpiresAt   time.Time
	CsrfTokenExpiresAt time.Time
	User               *User
	CreatedAt          time.Time `storm:"index"`
}

func (c *PoliteiaConfig) saveCookies(cookies []*http.Cookie) error {
	err := configDB.UpdateField(&PoliteiaConfig{ID: 1}, "Cookies", cookies)
	if err != nil {
		return fmt.Errorf("error saving cookies: %s", err.Error())
	}
	c.Cookies = cookies

	return nil
}

func (c *PoliteiaConfig) saveSession(cookies []*http.Cookie, user *User) error {
	cfg := &PoliteiaConfig{
		ID:               1,
		User:             user,
		Cookies:          cookies,
		SessionExpiresAt: time.Now().Add(time.Second * time.Duration(user.SessionMaxAge)),
	}

	err := configDB.Update(cfg)
	if err != nil {
		return fmt.Errorf("error saving user session: %s", err.Error())
	}

	c.Cookies = cookies
	c.User = user
	c.SessionExpiresAt = cfg.SessionExpiresAt

	return nil
}

func (c *PoliteiaConfig) saveCSRFToken(token string) error {
	err := configDB.UpdateField(&PoliteiaConfig{ID: 1}, "CsrfToken", token)
	if err != nil {
		return fmt.Errorf("error saving csrf token: %s", err.Error())
	}
	c.CsrfToken = token

	return nil
}

func (c *PoliteiaConfig) getCookies() []*http.Cookie {
	return c.Cookies
}
