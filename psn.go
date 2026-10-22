package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	routing "github.com/qiangxue/fasthttp-routing"
)

// PSNLink records a PlayStation Network account association.
type PSNLink struct {
	UserID    string    `json:"user_id"`
	AccountID string    `json:"account_id"`
	OnlineID  string    `json:"online_id"`
	LinkedAt  time.Time `json:"linked_at"`
}

func psnClientID() string     { return os.Getenv("PSN_CLIENT_ID") }
func psnClientSecret() string { return os.Getenv("PSN_CLIENT_SECRET") }
func psnRedirectURI() string  { return appBaseURL() + "/p/callback" }

const (
	psnAuthURL    = "https://ca.account.sony.com/api/authz/v3/oauth/authorize"
	psnTokenURL   = "https://ca.account.sony.com/api/authz/v3/oauth/token"
	psnProfileURL = "https://m.np.playstation.com/api/userProfile/v1/internal/users/me/profiles"
)

// psnAuth redirects the authenticated user to Sony's OAuth consent screen.
// GET /p/auth  (requires auth)
//
// PSN_CLIENT_ID and PSN_CLIENT_SECRET require enrollment in the PlayStation
// Partner Program (https://partners.playstation.net). Without credentials the
// endpoint returns 503.
func psnAuth(c *routing.Context) error {
	if psnClientID() == "" {
		c.SetStatusCode(503)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"PSN_CLIENT_ID not configured"}`)
		return nil
	}
	userID, _ := c.UserValue(authenticatedUserIDKey{}).(string)
	params := url.Values{}
	params.Set("access_type", "offline")
	params.Set("client_id", psnClientID())
	params.Set("redirect_uri", psnRedirectURI())
	params.Set("response_type", "code")
	params.Set("scope", "psn:mobile.v2.core psn:clientapp")
	params.Set("state", newOAuthState(userID))
	c.SetStatusCode(302)
	c.Response.Header.Set("Location", psnAuthURL+"?"+params.Encode())
	return nil
}

// psnCallback handles Sony's OAuth redirect and stores the PSN link.
// GET /p/callback
func psnCallback(c *routing.Context) error {
	if errParam := string(c.QueryArgs().Peek("error")); errParam != "" {
		writeJSONError(c, 400, "PSN OAuth denied: "+errParam)
		return nil
	}
	code := string(c.QueryArgs().Peek("code"))
	state := string(c.QueryArgs().Peek("state"))
	if code == "" || state == "" {
		c.SetStatusCode(400)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"missing code or state"}`)
		return nil
	}

	userID, err := verifyOAuthState(state)
	if err != nil {
		writeJSONError(c, 400, "invalid state: "+err.Error())
		return nil
	}

	accessToken, err := psnExchangeCode(code)
	if err != nil {
		writeJSONError(c, 502, "PSN token exchange failed: "+err.Error())
		return nil
	}

	onlineID, accountID, err := psnGetProfile(accessToken)
	if err != nil {
		writeJSONError(c, 502, "PSN profile lookup failed: "+err.Error())
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	_, err = db.Exec(ctx, `
		INSERT INTO psn_links (user_id, account_id, online_id, linked_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET
			account_id = $2,
			online_id  = $3,
			linked_at  = $4
	`, userID, accountID, onlineID, time.Now().UTC())
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to save PSN link"}`)
		return nil
	}

	c.SetContentType("application/json")
	out, _ := json.Marshal(map[string]interface{}{"success": true, "online_id": onlineID})
	c.Write(out)
	return nil
}

// psnExchangeCode exchanges the PSN authorization code for an access token.
func psnExchangeCode(code string) (string, error) {
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", code)
	body.Set("redirect_uri", psnRedirectURI())

	req, _ := http.NewRequest("POST", psnTokenURL, bytes.NewBufferString(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(psnClientID(), psnClientSecret())
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err = json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("failed to parse PSN token response")
	}
	token, _ := result["access_token"].(string)
	if token == "" {
		return "", fmt.Errorf("no access_token in PSN token response")
	}
	return token, nil
}

// psnGetProfile fetches the PSN Online ID and account ID using the access token.
func psnGetProfile(accessToken string) (onlineID, accountID string, err error) {
	req, _ := http.NewRequest("GET", psnProfileURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err = json.Unmarshal(data, &result); err != nil {
		return "", "", fmt.Errorf("failed to parse PSN profile response")
	}
	onlineID, _ = result["onlineId"].(string)
	accountID, _ = result["accountId"].(string)
	if onlineID == "" {
		return "", "", fmt.Errorf("onlineId not found in PSN profile response")
	}
	return onlineID, accountID, nil
}
