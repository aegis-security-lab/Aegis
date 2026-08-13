package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const sessionCookieName = "aegis_session"

type authService struct {
	passwordHash [sha256.Size]byte
	ttl          time.Duration
	mu           sync.Mutex
	sessions     map[[sha256.Size]byte]time.Time
}

func newAuthService(password string, ttl time.Duration) (*authService, error) {
	if strings.TrimSpace(password) == "" {
		return nil, errors.New("访问密码不能为空")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &authService{
		passwordHash: sha256.Sum256([]byte(password)),
		ttl:          ttl,
		sessions:     make(map[[sha256.Size]byte]time.Time),
	}, nil
}

func (a *authService) login(c *gin.Context) {
	var input struct {
		Password string `json:"password"`
	}
	if !bindJSON(c, &input) {
		return
	}
	candidate := sha256.Sum256([]byte(input.Password))
	if subtle.ConstantTimeCompare(candidate[:], a.passwordHash[:]) != 1 {
		writeError(c, http.StatusUnauthorized, errors.New("密码错误"))
		return
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeError(c, http.StatusInternalServerError, errors.New("无法创建登录会话"))
		return
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	expiresAt := time.Now().Add(a.ttl)
	a.mu.Lock()
	a.removeExpiredLocked(time.Now())
	a.sessions[sha256.Sum256([]byte(token))] = expiresAt
	a.mu.Unlock()
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(sessionCookieName, token, int(a.ttl.Seconds()), "/", "", c.Request.TLS != nil, true)
	c.JSON(http.StatusOK, gin.H{"expiresAt": expiresAt})
}

func (a *authService) logout(c *gin.Context) {
	token := accessToken(c.Request)
	if token == "" {
		if cookie, err := c.Cookie(sessionCookieName); err == nil {
			token = strings.TrimSpace(cookie)
		}
	}
	if token != "" {
		a.mu.Lock()
		delete(a.sessions, sha256.Sum256([]byte(token)))
		a.mu.Unlock()
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(sessionCookieName, "", -1, "/", "", c.Request.TLS != nil, true)
	c.Status(http.StatusNoContent)
}

func (a *authService) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := accessToken(c.Request)
		if token == "" {
			if cookie, err := c.Cookie(sessionCookieName); err == nil {
				token = strings.TrimSpace(cookie)
			}
		}
		if !a.valid(token, time.Now()) {
			c.Header("WWW-Authenticate", "Bearer")
			writeError(c, http.StatusUnauthorized, errors.New("请先登录"))
			return
		}
		c.Next()
	}
}

func (a *authService) valid(token string, now time.Time) bool {
	if token == "" {
		return false
	}
	hash := sha256.Sum256([]byte(token))
	a.mu.Lock()
	defer a.mu.Unlock()
	expiresAt, ok := a.sessions[hash]
	if !ok || !expiresAt.After(now) {
		delete(a.sessions, hash)
		return false
	}
	return true
}

func (a *authService) removeExpiredLocked(now time.Time) {
	for token, expiresAt := range a.sessions {
		if !expiresAt.After(now) {
			delete(a.sessions, token)
		}
	}
}

func accessToken(request *http.Request) string {
	const prefix = "Bearer "
	value := strings.TrimSpace(request.Header.Get("Authorization"))
	if len(value) < len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(value[len(prefix):])
}
