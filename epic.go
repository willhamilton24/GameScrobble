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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EpicLink records an Epic Games account association.
type EpicLink struct {
	UserID      string    `bson:"user_id" json:"user_id"`
	EpicID      string    `bson:"epic_id" json:"epic_id"`
	DisplayName string    `bson:"display_name" json:"display_name"`
	LinkedAt    time.Time `bson:"linked_at" json:"linked_at"`
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
		c.SetStatusCode(400)
		c.SetContentType("application/json")
		fmt.Fprintf(c, `{"error":"Epic OAuth denied: %s"}`, errParam)
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
		c.SetStatusCode(400)
		c.SetContentType("application/json")
		fmt.Fprintf(c, `{"error":"invalid state: %s"}`, err.Error())
		return nil
	}

	accessToken, err := epicExchangeCode(code)
	if err != nil {
		c.SetStatusCode(502)
		c.SetContentType("application/json")
		fmt.Fprintf(c, `{"error":"Epic token exchange failed: %s"}`, err.Error())
		return nil
	}

	epicID, displayName, err := epicGetUserInfo(accessToken)
	if err != nil {
		c.SetStatusCode(502)
		c.SetContentType("application/json")
		fmt.Fprintf(c, `{"error":"Epic user info lookup failed: %s"}`, err.Error())
		return nil
	}

	link := EpicLink{UserID: userID, EpicID: epicID, DisplayName: displayName, LinkedAt: time.Now().UTC()}
	ctx, cancel := context.WithTimeout(context.Background(), mongoUpdateTimeout)
	defer cancel()
	if _, err = mongoClient.Database("Gamium").Collection("epic_links").UpdateOne(ctx,
		bson.D{{Key: "user_id", Value: userID}},
		bson.D{{Key: "$set", Value: link}},
		options.Update().SetUpsert(true),
	); err != nil {
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
