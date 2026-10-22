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

// XboxLink records an Xbox Live account association.
type XboxLink struct {
	UserID   string    `json:"user_id"`
	XUID     string    `json:"xuid"`
	Gamertag string    `json:"gamertag"`
	LinkedAt time.Time `json:"linked_at"`
}

func xboxClientID() string     { return os.Getenv("XBOX_CLIENT_ID") }
func xboxClientSecret() string { return os.Getenv("XBOX_CLIENT_SECRET") }
func xboxRedirectURI() string  { return appBaseURL() + "/x/callback" }

const (
	xboxMSAuthURL   = "https://login.live.com/oauth20_authorize.srf"
	xboxMSTokenURL  = "https://login.live.com/oauth20_token.srf"
	xboxLiveAuthURL = "https://user.auth.xboxlive.com/user/authenticate"
	xboxXSTSURL     = "https://xsts.auth.xboxlive.com/xsts/authorize"
)

// xboxAuth redirects the authenticated user to Microsoft's OAuth consent screen.
// GET /x/auth  (requires auth)
func xboxAuth(c *routing.Context) error {
	if xboxClientID() == "" {
		c.SetStatusCode(503)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"XBOX_CLIENT_ID not configured"}`)
		return nil
	}
	userID, _ := c.UserValue(authenticatedUserIDKey{}).(string)
	params := url.Values{}
	params.Set("client_id", xboxClientID())
	params.Set("response_type", "code")
	params.Set("redirect_uri", xboxRedirectURI())
	params.Set("scope", "Xboxlive.signin Xboxlive.offline_access")
	params.Set("state", newOAuthState(userID))
	c.SetStatusCode(302)
	c.Response.Header.Set("Location", xboxMSAuthURL+"?"+params.Encode())
	return nil
}

// xboxCallback handles Microsoft's OAuth redirect and stores the Xbox Live link.
// GET /x/callback
func xboxCallback(c *routing.Context) error {
	if errParam := string(c.QueryArgs().Peek("error")); errParam != "" {
		writeJSONError(c, 400, "Xbox OAuth denied: "+errParam)
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

	msToken, err := xboxExchangeCode(code)
	if err != nil {
		writeJSONError(c, 502, "token exchange failed: "+err.Error())
		return nil
	}

	gamertag, xuid, err := xboxGetIdentity(msToken)
	if err != nil {
		writeJSONError(c, 502, "Xbox identity lookup failed: "+err.Error())
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	_, err = db.Exec(ctx, `
		INSERT INTO xbox_links (user_id, xuid, gamertag, linked_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET
			xuid      = $2,
			gamertag  = $3,
			linked_at = $4
	`, userID, xuid, gamertag, time.Now().UTC())
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to save Xbox link"}`)
		return nil
	}

	c.SetContentType("application/json")
	out, _ := json.Marshal(map[string]interface{}{"success": true, "gamertag": gamertag, "xuid": xuid})
	c.Write(out)
	return nil
}

// xboxExchangeCode exchanges the Microsoft authorization code for an access token.
func xboxExchangeCode(code string) (string, error) {
	body := url.Values{}
	body.Set("client_id", xboxClientID())
	body.Set("client_secret", xboxClientSecret())
	body.Set("code", code)
	body.Set("grant_type", "authorization_code")
	body.Set("redirect_uri", xboxRedirectURI())

	resp, err := oauthHTTPClient.Post(xboxMSTokenURL, "application/x-www-form-urlencoded",
		bytes.NewBufferString(body.Encode()))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err = json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("failed to parse MS token response")
	}
	token, _ := result["access_token"].(string)
	if token == "" {
		return "", fmt.Errorf("no access_token in MS token response")
	}
	return token, nil
}

// xboxGetIdentity exchanges a Microsoft access token for Xbox Live gamertag and XUID
// via the XBL → XSTS token exchange chain.
func xboxGetIdentity(msAccessToken string) (gamertag, xuid string, err error) {
	// Step 1: exchange MS token for XBL token.
	xblBody, _ := json.Marshal(map[string]interface{}{
		"Properties": map[string]interface{}{
			"AuthMethod": "RPS",
			"SiteName":   "user.auth.xboxlive.com",
			"RpsTicket":  "d=" + msAccessToken,
		},
		"RelyingParty": "http://auth.xboxlive.com",
		"TokenType":    "JWT",
	})
	xblReq, _ := http.NewRequest("POST", xboxLiveAuthURL, bytes.NewReader(xblBody))
	xblReq.Header.Set("Content-Type", "application/json")
	xblReq.Header.Set("Accept", "application/json")
	xblResp, err := oauthHTTPClient.Do(xblReq)
	if err != nil {
		return "", "", fmt.Errorf("XBL auth request failed: %w", err)
	}
	defer xblResp.Body.Close()
	xblData, _ := io.ReadAll(xblResp.Body)

	var xblResult map[string]interface{}
	json.Unmarshal(xblData, &xblResult)
	xblToken, _ := xblResult["Token"].(string)
	if xblToken == "" {
		return "", "", fmt.Errorf("XBL token not returned (status %d)", xblResp.StatusCode)
	}

	// Step 2: exchange XBL token for XSTS token (contains gamertag + xuid in claims).
	xstsBody, _ := json.Marshal(map[string]interface{}{
		"Properties": map[string]interface{}{
			"SandboxId":  "RETAIL",
			"UserTokens": []string{xblToken},
		},
		"RelyingParty": "http://xboxlive.com",
		"TokenType":    "JWT",
	})
	xstsReq, _ := http.NewRequest("POST", xboxXSTSURL, bytes.NewReader(xstsBody))
	xstsReq.Header.Set("Content-Type", "application/json")
	xstsReq.Header.Set("Accept", "application/json")
	xstsResp, err := oauthHTTPClient.Do(xstsReq)
	if err != nil {
		return "", "", fmt.Errorf("XSTS auth request failed: %w", err)
	}
	defer xstsResp.Body.Close()
	xstsData, _ := io.ReadAll(xstsResp.Body)

	var xstsResult map[string]interface{}
	json.Unmarshal(xstsData, &xstsResult)

	if dc, ok := xstsResult["DisplayClaims"].(map[string]interface{}); ok {
		if xui, ok := dc["xui"].([]interface{}); ok && len(xui) > 0 {
			if xuiMap, ok := xui[0].(map[string]interface{}); ok {
				xuid, _ = xuiMap["xid"].(string)
				gamertag, _ = xuiMap["gtg"].(string)
			}
		}
	}
	if xuid == "" || gamertag == "" {
		return "", "", fmt.Errorf("missing identity claims in XSTS response (status %d)", xstsResp.StatusCode)
	}
	return gamertag, xuid, nil
}
