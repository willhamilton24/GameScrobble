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

// EpicLink records an Epic Games account association.
type EpicLink struct {
	UserID      string    `json:"user_id"`
	EpicID      string    `json:"epic_id"`
	DisplayName string    `json:"display_name"`
	LinkedAt    time.Time `json:"linked_at"`
}

func epicClientID() string     { return os.Getenv("EPIC_CLIENT_ID") }
func epicClientSecret() string { return os.Getenv("EPIC_CLIENT_SECRET") }
func epicRedirectURI() string  { return appBaseURL() + "/e/callback" }

const (
	epicAuthURL     = "https://www.epicgames.com/id/authorize"
	epicTokenURL    = "https://api.epicgames.dev/epic/oauth/v2/token"
	epicUserInfoURL = "https://api.epicgames.dev/epic/oauth/v2/userInfo"
)

// epicAuth redirects the authenticated user to Epic Games' OAuth consent screen.
// GET /e/auth  (requires auth)
func epicAuth(c *routing.Context) error {
	if epicClientID() == "" {
		c.SetStatusCode(503)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"EPIC_CLIENT_ID not configured"}`)
		return nil
	}
	userID, _ := c.UserValue(authenticatedUserIDKey{}).(string)
	params := url.Values{}
	params.Set("client_id", epicClientID())
	params.Set("response_type", "code")
	params.Set("redirect_uri", epicRedirectURI())
	params.Set("scope", "basic_profile")
	params.Set("state", newOAuthState(userID))
	c.SetStatusCode(302)
	c.Response.Header.Set("Location", epicAuthURL+"?"+params.Encode())
	return nil
}

// epicCallback handles Epic Games' OAuth redirect and stores the Epic link.
// GET /e/callback
func epicCallback(c *routing.Context) error {
	if errParam := string(c.QueryArgs().Peek("error")); errParam != "" {
		writeJSONError(c, 400, "Epic OAuth denied: "+errParam)
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

	accessToken, err := epicExchangeCode(code)
	if err != nil {
		writeJSONError(c, 502, "Epic token exchange failed: "+err.Error())
		return nil
	}

	epicID, displayName, err := epicGetUserInfo(accessToken)
	if err != nil {
		writeJSONError(c, 502, "Epic user info lookup failed: "+err.Error())
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	_, err = db.Exec(ctx, `
		INSERT INTO epic_links (user_id, epic_id, display_name, linked_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET
			epic_id      = $2,
			display_name = $3,
			linked_at    = $4
	`, userID, epicID, displayName, time.Now().UTC())
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to save Epic link"}`)
		return nil
	}

	c.SetContentType("application/json")
	out, _ := json.Marshal(map[string]interface{}{"success": true, "display_name": displayName, "epic_id": epicID})
	c.Write(out)
	return nil
}

// epicExchangeCode exchanges the Epic Games authorization code for an access token.
func epicExchangeCode(code string) (string, error) {
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", code)
	body.Set("redirect_uri", epicRedirectURI())

	req, _ := http.NewRequest("POST", epicTokenURL, bytes.NewBufferString(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(epicClientID(), epicClientSecret())
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err = json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("failed to parse Epic token response")
	}
	token, _ := result["access_token"].(string)
	if token == "" {
		return "", fmt.Errorf("no access_token in Epic token response")
	}
	return token, nil
}

// epicGetUserInfo fetches the Epic account ID and display name via the userInfo endpoint.
func epicGetUserInfo(accessToken string) (epicID, displayName string, err error) {
	req, _ := http.NewRequest("GET", epicUserInfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err = json.Unmarshal(data, &result); err != nil {
		return "", "", fmt.Errorf("failed to parse Epic userInfo response")
	}
	// Epic userInfo: "sub" = account ID, "preferred_username" = display name
	epicID, _ = result["sub"].(string)
	displayName, _ = result["preferred_username"].(string)
	if epicID == "" {
		return "", "", fmt.Errorf("Epic account ID not found in userInfo response")
	}
	return epicID, displayName, nil
}
