package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	routing "github.com/qiangxue/fasthttp-routing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

// User is a registered account stored in the users table.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// oauthHTTPClient is shared by all OAuth provider helpers.
var oauthHTTPClient = &http.Client{Timeout: 10 * time.Second}

func jwtSecret() []byte {
	return []byte(os.Getenv("JWT_SECRET"))
}

// writeJSONError writes a JSON error response with the message safely encoded.
func writeJSONError(c *routing.Context, status int, msg string) {
	c.SetStatusCode(status)
	c.SetContentType("application/json")
	out, _ := json.Marshal(map[string]string{"error": msg})
	c.Write(out)
}

// newUserID returns a random 32-character hex string for use as a user ID.
func newUserID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate user ID: " + err.Error())
	}
	return fmt.Sprintf("%x", b)
}

// signToken creates a signed HS256 JWT with a 30-day expiry.
func signToken(userID string) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claimsJSON, _ := json.Marshal(map[string]interface{}{
		"sub": userID,
		"exp": time.Now().Add(30 * 24 * time.Hour).Unix(),
	})
	payload := header + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	mac := hmac.New(sha256.New, jwtSecret())
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// verifyToken validates a JWT and returns the user ID.
func verifyToken(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid token format")
	}
	payload := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, jwtSecret())
	mac.Write([]byte(payload))
	if !hmac.Equal([]byte(parts[2]), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))) {
		return "", fmt.Errorf("invalid signature")
	}
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid claims encoding")
	}
	var claims map[string]interface{}
	if err = json.Unmarshal(claimsBytes, &claims); err != nil {
		return "", fmt.Errorf("invalid claims")
	}
	if exp, _ := claims["exp"].(float64); time.Now().Unix() > int64(exp) {
		return "", fmt.Errorf("token expired")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", fmt.Errorf("missing subject")
	}
	return sub, nil
}

// newOAuthState creates a short-lived signed state for OAuth CSRF protection.
// The state encodes the user ID and a timestamp, signed with the JWT secret.
func newOAuthState(userID string) string {
	raw := base64.RawURLEncoding.EncodeToString(
		[]byte(userID + "|" + strconv.FormatInt(time.Now().Unix(), 10)),
	)
	mac := hmac.New(sha256.New, jwtSecret())
	mac.Write([]byte(raw))
	return raw + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verifyOAuthState validates an OAuth state parameter and returns the user ID.
// States expire after 10 minutes.
func verifyOAuthState(state string) (string, error) {
	idx := strings.LastIndex(state, ".")
	if idx < 0 {
		return "", fmt.Errorf("invalid state format")
	}
	raw, sig := state[:idx], state[idx+1:]
	mac := hmac.New(sha256.New, jwtSecret())
	mac.Write([]byte(raw))
	if !hmac.Equal([]byte(sig), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))) {
		return "", fmt.Errorf("invalid state signature")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("invalid state encoding")
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("malformed state")
	}
	ts, _ := strconv.ParseInt(parts[1], 10, 64)
	if time.Now().Unix()-ts > 600 {
		return "", fmt.Errorf("state expired")
	}
	return parts[0], nil
}

// authMiddleware reads the Bearer token, validates it, and sets authenticatedUserIDKey.
// Returns false and writes a 401 response when authentication fails; returns true on success.
func authMiddleware(c *routing.Context) bool {
	authHeader := string(c.Request.Header.Peek("Authorization"))
	if !strings.HasPrefix(authHeader, "Bearer ") {
		c.SetStatusCode(401)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"authentication required"}`)
		return false
	}
	userID, err := verifyToken(authHeader[7:])
	if err != nil {
		c.SetStatusCode(401)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"invalid or expired token"}`)
		return false
	}
	c.SetUserValue(authenticatedUserIDKey{}, userID)
	return true
}

// registerUser handles POST /auth/register.
// Body: {"email":"...","password":"...","source":"..."}
func registerUser(c *routing.Context) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Source   string `json:"source"`
	}
	if err := json.Unmarshal(c.PostBody(), &req); err != nil || req.Email == "" || req.Password == "" {
		c.SetStatusCode(400)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"email and password are required"}`)
		return nil
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if len(req.Password) < 8 {
		c.SetStatusCode(400)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"password must be at least 8 characters"}`)
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to hash password"}`)
		return nil
	}

	id := newUserID()
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	// Sanitize source: keep only safe chars, max 64.
	source := req.Source
	if len(source) > 64 {
		source = source[:64]
	}

	_, err = db.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, created_at, signup_source) VALUES ($1, $2, $3, $4, $5)`,
		id, req.Email, string(hash), time.Now().UTC(), source,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			c.SetStatusCode(409)
			c.SetContentType("application/json")
			c.WriteString(`{"error":"email already registered"}`)
			return nil
		}
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to create user"}`)
		return nil
	}

	token, err := signToken(id)
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to sign token"}`)
		return nil
	}

	c.SetStatusCode(201)
	c.SetContentType("application/json")
	out, _ := json.Marshal(map[string]interface{}{
		"token": token,
		"user":  map[string]string{"id": id, "email": req.Email},
	})
	c.Write(out)
	return nil
}

// loginUser handles POST /auth/login.
// Body: {"email":"...","password":"..."}
func loginUser(c *routing.Context) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(c.PostBody(), &req); err != nil || req.Email == "" || req.Password == "" {
		c.SetStatusCode(400)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"email and password are required"}`)
		return nil
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	var user User
	err := db.QueryRow(ctx, `SELECT id, email, password_hash FROM users WHERE email = $1`, req.Email).
		Scan(&user.ID, &user.Email, &user.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		c.SetStatusCode(401)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"invalid email or password"}`)
		return nil
	}
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"database error"}`)
		return nil
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.SetStatusCode(401)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"invalid email or password"}`)
		return nil
	}

	// Best-effort; login still succeeds if this write fails.
	loginCtx, loginCancel := context.WithTimeout(context.Background(), dbTimeout)
	db.Exec(loginCtx, `UPDATE users SET last_login_at = $1 WHERE id = $2`, time.Now().UTC(), user.ID)
	loginCancel()

	token, err := signToken(user.ID)
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to sign token"}`)
		return nil
	}

	c.SetContentType("application/json")
	out, _ := json.Marshal(map[string]interface{}{
		"token": token,
		"user":  map[string]string{"id": user.ID, "email": user.Email},
	})
	c.Write(out)
	return nil
}

// getMe handles GET /auth/me — returns the authenticated user's profile.
func getMe(c *routing.Context) error {
	userID, _ := c.UserValue(authenticatedUserIDKey{}).(string)

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	var user User
	err := db.QueryRow(ctx, `SELECT id, email FROM users WHERE id = $1`, userID).
		Scan(&user.ID, &user.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		c.SetStatusCode(404)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"user not found"}`)
		return nil
	}
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"database error"}`)
		return nil
	}

	c.SetContentType("application/json")
	out, _ := json.Marshal(map[string]string{"id": user.ID, "email": user.Email})
	c.Write(out)
	return nil
}
