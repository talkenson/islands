package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Claims struct {
	UserID  uint64 `json:"user_id"`
	ActorID uint64 `json:"actor_id"`
	WorldID uint64 `json:"world_id"`
	Role    string `json:"role,omitempty"`
	Expires int64  `json:"exp"`
}

type Manager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewManager(secret string, ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Manager{
		secret: []byte(secret),
		ttl:    ttl,
		now:    time.Now,
	}
}

func (m *Manager) Issue(userID, actorID, worldID uint64) (string, Claims, error) {
	if len(m.secret) == 0 {
		return "", Claims{}, errors.New("auth secret is empty")
	}
	claims := Claims{
		UserID:  userID,
		ActorID: actorID,
		WorldID: worldID,
		Expires: m.now().Add(m.ttl).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", Claims{}, err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	signature := m.sign(body)
	return body + "." + signature, claims, nil
}

func (m *Manager) Verify(token string) (Claims, error) {
	if len(m.secret) == 0 {
		return Claims{}, errors.New("auth secret is empty")
	}
	body, signature, ok := strings.Cut(token, ".")
	if !ok || body == "" || signature == "" {
		return Claims{}, errors.New("invalid token shape")
	}
	if !hmac.Equal([]byte(signature), []byte(m.sign(body))) {
		return Claims{}, errors.New("invalid token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return Claims{}, err
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, err
	}
	if claims.UserID == 0 || claims.ActorID == 0 || claims.WorldID == 0 {
		return Claims{}, errors.New("token is missing required claims")
	}
	if claims.Expires <= m.now().Unix() {
		return Claims{}, errors.New("token expired")
	}
	return claims, nil
}

func (m *Manager) sign(body string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type contextKey struct{}

func WithClaims(ctx context.Context, claims Claims) context.Context {
	return context.WithValue(ctx, contextKey{}, claims)
}

func FromContext(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(contextKey{}).(Claims)
	return claims, ok
}

func Middleware(manager *Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" {
				if cookie, err := r.Cookie("islands_session"); err == nil {
					token = cookie.Value
				}
			}
			if token == "" {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "missing auth token")
				return
			}
			claims, err := manager.Verify(token)
			if err != nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid auth token")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
		})
	}
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	scheme, token, ok := strings.Cut(auth, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func WorldIDFromPath(path, prefix, suffix string) (uint64, bool) {
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return 0, false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	id, err := strconv.ParseUint(strings.Trim(raw, "/"), 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Code: code, Message: message})
}

func RequireWorld(claims Claims, worldID uint64) error {
	if claims.WorldID != worldID {
		return fmt.Errorf("actor is not authorized for world %d", worldID)
	}
	return nil
}
