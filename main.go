package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	routing "github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

const steamRequestTimeout = 5 * time.Second

var steamHTTPGet = fasthttp.GetTimeout

// Only authMiddleware (auth.go) may set this context key.
type authenticatedUserIDKey struct{}

type Offer struct {
	Vendor     string `json:"vendor"`
	Digital    bool   `json:"digital"`
	Platform   string `json:"platform"`
	Url        string `json:"url"`
	InternalId string `json:"internal_id"`
}

type Game struct {
	Title             string   `json:"title"`
	Img               string   `json:"img"`
	Dev               string   `json:"dev"`
	Pub               string   `json:"pub"`
	RDate             string   `json:"rdate"`
	Metacritic        string   `json:"metacritic"`
	PcOffers          []Offer  `json:"pc_offers"`
	XboxOffers        []Offer  `json:"xbox_offers"`
	PlaystationOffers []Offer  `json:"playstation_offers"`
	SwitchOffers      []Offer  `json:"switch_offers"`
	Platforms         []string `json:"platforms"`
}

type DLC struct {
	game              Game
	title             string
	img               string
	rDate             string
	pcOffers          []Offer
	xboxOffers        []Offer
	playstationOffers []Offer
	switchOffers      []Offer
}

type SteamResult struct {
	Name string
}

// SteamLink records an unverified association; public profile lookup does not prove ownership.
type SteamLink struct {
	UserID            string `json:"user_id"`
	SteamID           string `json:"steam_id"`
	OwnershipVerified bool   `json:"ownership_verified"`
}

// PlayActivity records a daily playtime delta used to build the public profile heatmap.
type PlayActivity struct {
	UserID  string
	AppID   int
	Name    string
	Date    string // YYYY-MM-DD
	Minutes int
}

// PublicGame is one entry in the public profile year-in-games list.
type PublicGame struct {
	AppID   int    `json:"app_id"`
	Name    string `json:"name"`
	Minutes int    `json:"minutes"`
}

// HeatmapDay is one data point in the public profile activity heatmap.
type HeatmapDay struct {
	Date    string `json:"date"`
	Minutes int    `json:"minutes"`
}

// PlayHistoryStat is one entry in the deep-stats response.
type PlayHistoryStat struct {
	AppID          int       `json:"app_id"`
	Name           string    `json:"name"`
	PlaytimeTotal  int       `json:"playtime_total"`
	PlaytimeRecent int       `json:"playtime_recent"`
	LastPolled     time.Time `json:"last_polled"`
}

//
// Steam Pricing
//

func readSteamDLC(i int, g Game) string {
	var body []byte
	id := strconv.Itoa(i)

	status, resp, err := steamHTTPGet(body, "https://store.steampowered.com/api/appdetails?appids="+id, steamRequestTimeout)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println(status)

	title := fastjson.GetString(resp, id, "data", "name")
	fmt.Println(title)
	// Check Existance
	return ""
}

func getDLCArray(data []byte, appid string) []int {
	dlcs := []int{}
	x := 0
	for {
		if id := fastjson.GetInt(data, appid, "data", "dlc", strconv.Itoa(x)); id == 0 {
			break
		} else {
			dlcs = append(dlcs, id)
		}
		x++
	}
	return dlcs
}

func readSteamGame(i int) {
	var body []byte
	id := strconv.Itoa(i)

	status, resp, err := steamHTTPGet(body, "https://store.steampowered.com/api/appdetails?appids="+id, steamRequestTimeout)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println(status)

	title := fastjson.GetString(resp, id, "data", "name")
	fmt.Println(title)

	if fastjson.GetString(resp, id, "data", "type") == "game" {
		fmt.Println("Game")

		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()

		var existing string
		err = db.QueryRow(ctx, `SELECT title FROM games WHERE title = $1`, title).Scan(&existing)
		if errors.Is(err, pgx.ErrNoRows) {
			fmt.Println("NOT SAVED")

			newGame := Game{
				Title:             title,
				Img:               fastjson.GetString(resp, id, "data", "header_image"),
				Dev:               fastjson.GetString(resp, id, "data", "developers", "0"),
				Pub:               fastjson.GetString(resp, id, "data", "publishers", "0"),
				RDate:             fastjson.GetString(resp, id, "data", "release_date", "date"),
				Metacritic:        fastjson.GetString(resp, id, "data", "metacritic", "url"),
				PcOffers:          []Offer{},
				XboxOffers:        []Offer{},
				PlaystationOffers: []Offer{},
				SwitchOffers:      []Offer{},
				Platforms:         []string{},
			}
			if fastjson.GetBool(resp, id, "data", "platforms", "windows") {
				newGame.Platforms = append(newGame.Platforms, "windows")
			}
			if fastjson.GetBool(resp, id, "data", "platforms", "mac") {
				newGame.Platforms = append(newGame.Platforms, "mac")
			}
			if fastjson.GetBool(resp, id, "data", "platforms", "linux") {
				newGame.Platforms = append(newGame.Platforms, "linux")
			}

			appid := strconv.Itoa(fastjson.GetInt(resp, id, "data", "steam_appid"))
			newOffer := Offer{
				Vendor:     "steam",
				Digital:    true,
				Platform:   "pc",
				Url:        "https://store.steampowered.com/app/" + appid + "/",
				InternalId: appid,
			}
			fmt.Println(newOffer)
			newGame.PcOffers = append(newGame.PcOffers, newOffer)
			fmt.Println(newGame)

			pcJSON, _ := json.Marshal(newGame.PcOffers)
			platformsJSON, _ := json.Marshal(newGame.Platforms)

			_, err2 := db.Exec(ctx, `
				INSERT INTO games (title, img, dev, pub, rdate, metacritic, pc_offers, xbox_offers, playstation_offers, switch_offers, platforms)
				VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, $8::jsonb)
				ON CONFLICT (title) DO NOTHING
			`, newGame.Title, newGame.Img, newGame.Dev, newGame.Pub, newGame.RDate, newGame.Metacritic,
				string(pcJSON), string(platformsJSON))
			if err2 != nil {
				log.Printf("readSteamGame: insert error for %q: %v", title, err2)
			} else {
				fmt.Println("Inserted game:", title)
			}
		} else if err != nil {
			fmt.Println(err.Error())
		} else {
			fmt.Println(existing)
		}
	}
}

func scrapeSteam(latency time.Duration) {
	for i := 0; i < 1000000; i += 10 {
		readSteamGame(i)
		time.Sleep(latency * time.Millisecond)
	}
}

//
// Steam Accounts — official Web API (api.steampowered.com)
//

// steamIDPattern matches a Steam64 ID: exactly 17 digits.
var steamIDPattern = regexp.MustCompile(`^[0-9]{17}$`)

func steamAPIKey() string {
	return os.Getenv("STEAM_API_KEY")
}

// steamGet performs a GET against the Steam Web API and returns the response body.
func steamGet(path string) ([]byte, error) {
	var body []byte
	status, resp, err := steamHTTPGet(body, "https://api.steampowered.com"+path, steamRequestTimeout)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("Steam API returned status %d", status)
	}
	return resp, nil
}

// getCurrentGame returns the game a Steam user is currently playing (if any).
// Query param: steam_id (Steam64 ID)
func getCurrentGame(c *routing.Context) error {
	steamID := string(c.QueryArgs().Peek("steam_id"))
	if !steamIDPattern.MatchString(steamID) {
		c.SetStatusCode(400)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"steam_id must be a 17-digit Steam64 ID"}`)
		return nil
	}

	key := steamAPIKey()
	if key == "" {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"STEAM_API_KEY not configured"}`)
		return nil
	}

	resp, err := steamGet("/ISteamUser/GetPlayerSummaries/v0002/?key=" + key + "&steamids=" + steamID)
	if err != nil {
		c.SetStatusCode(502)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"Steam API request failed"}`)
		return nil
	}

	v, err := fastjson.ParseBytes(resp)
	if err != nil {
		c.SetStatusCode(502)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to parse Steam API response"}`)
		return nil
	}

	players := v.GetArray("response", "players")
	c.SetContentType("application/json")
	if len(players) == 0 {
		c.WriteString(`{"error":"Steam account not found"}`)
		return nil
	}

	player := players[0]
	gameName := string(player.GetStringBytes("gameextrainfo"))
	gameID := string(player.GetStringBytes("gameid"))

	if gameName == "" {
		c.WriteString(`{"playing":false}`)
	} else {
		out, _ := json.Marshal(map[string]interface{}{
			"playing": true,
			"game":    gameName,
			"game_id": gameID,
		})
		c.Write(out)
	}
	return nil
}

// getRecentGames returns the last 10 games played on a Steam account.
// Query param: steam_id (Steam64 ID)
func getRecentGames(c *routing.Context) error {
	steamID := string(c.QueryArgs().Peek("steam_id"))
	if !steamIDPattern.MatchString(steamID) {
		c.SetStatusCode(400)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"steam_id must be a 17-digit Steam64 ID"}`)
		return nil
	}

	key := steamAPIKey()
	if key == "" {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"STEAM_API_KEY not configured"}`)
		return nil
	}

	resp, err := steamGet("/IPlayerService/GetRecentlyPlayedGames/v0001/?key=" + key + "&steamid=" + steamID + "&count=10&format=json")
	if err != nil {
		c.SetStatusCode(502)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"Steam API request failed"}`)
		return nil
	}

	v, err := fastjson.ParseBytes(resp)
	if err != nil {
		c.SetStatusCode(502)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to parse Steam API response"}`)
		return nil
	}

	games := v.Get("response", "games")
	c.SetContentType("application/json")
	if games == nil {
		c.WriteString(`{"games":[]}`)
	} else {
		fmt.Fprintf(c, `{"games":%s}`, games.String())
	}
	return nil
}

// linkAccount stores an unverified Steam association for an authenticated user.
// Body: { "steam_id": "76561198..." }; an optional user_id must match the session.
func linkAccount(c *routing.Context) error {
	userID, _ := c.UserValue(authenticatedUserIDKey{}).(string)
	if userID == "" {
		c.SetStatusCode(403)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"Steam linking is disabled until server authentication is available"}`)
		return nil
	}
	var link SteamLink
	if err := json.Unmarshal(c.PostBody(), &link); err != nil || !steamIDPattern.MatchString(link.SteamID) {
		c.SetStatusCode(400)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"request body must be JSON with a 17-digit steam_id"}`)
		return nil
	}

	if link.UserID != "" && link.UserID != userID {
		c.SetStatusCode(403)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"cannot modify another user's Steam link"}`)
		return nil
	}
	link.UserID = userID
	link.OwnershipVerified = false

	key := steamAPIKey()
	if key == "" {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"STEAM_API_KEY not configured"}`)
		return nil
	}

	// Check existence only: a public Steam profile does not prove ownership.
	resp, err := steamGet("/ISteamUser/GetPlayerSummaries/v0002/?key=" + key + "&steamids=" + link.SteamID)
	if err != nil {
		c.SetStatusCode(502)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"could not look up Steam account"}`)
		return nil
	}

	v, err := fastjson.ParseBytes(resp)
	if err != nil {
		c.SetStatusCode(502)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to parse Steam API response"}`)
		return nil
	}
	players := v.GetArray("response", "players")
	if len(players) == 0 {
		c.SetStatusCode(404)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"Steam account not found"}`)
		return nil
	}

	personaName := string(players[0].GetStringBytes("personaname"))

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	_, err = db.Exec(ctx, `
		INSERT INTO steam_links (user_id, steam_id, ownership_verified)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET
			steam_id           = $2,
			ownership_verified = $3
	`, link.UserID, link.SteamID, link.OwnershipVerified)
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to save Steam link"}`)
		return nil
	}

	c.SetContentType("application/json")
	out, _ := json.Marshal(map[string]interface{}{
		"success":            true,
		"persona_name":       personaName,
		"ownership_verified": false,
	})
	c.Write(out)
	return nil
}

//
// Premium Routes
//

// getDeepStats returns all-time playtime aggregations for a user (premium only).
// Query param: user_id
func getDeepStats(c *routing.Context) error {
	userID := string(c.QueryArgs().Peek("user_id"))
	if userID == "" {
		c.SetStatusCode(400)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"user_id is required"}`)
		return nil
	}

	ok, err := premiums.checkPremium(c.RequestCtx, userID)
	if err != nil || !ok {
		c.SetStatusCode(402)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"premium subscription required","upgrade_url":"/api/stripe/checkout"}`)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `
		SELECT app_id, name, playtime_total, playtime_recent, last_polled
		FROM play_history
		WHERE user_id = $1
		ORDER BY playtime_total DESC
	`, userID)
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to fetch stats"}`)
		return nil
	}
	defer rows.Close()

	stats := []PlayHistoryStat{}
	for rows.Next() {
		var s PlayHistoryStat
		if err := rows.Scan(&s.AppID, &s.Name, &s.PlaytimeTotal, &s.PlaytimeRecent, &s.LastPolled); err != nil {
			c.SetStatusCode(500)
			c.SetContentType("application/json")
			c.WriteString(`{"error":"failed to fetch stats"}`)
			return nil
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to fetch stats"}`)
		return nil
	}

	c.SetContentType("application/json")
	out, _ := json.Marshal(map[string]interface{}{"stats": stats})
	c.Write(out)
	return nil
}

// exportRow holds one play-history entry for CSV export.
type exportRow struct {
	AppID          int
	Name           string
	PlaytimeTotal  int
	PlaytimeRecent int
	LastPolled     time.Time
}

// findExportHistory can be replaced in tests to avoid a live DB connection.
var findExportHistory = func(ctx context.Context, userID string) ([]exportRow, error) {
	rows, err := db.Query(ctx, `
		SELECT app_id, name, playtime_total, playtime_recent, last_polled
		FROM play_history
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []exportRow
	for rows.Next() {
		var r exportRow
		if err := rows.Scan(&r.AppID, &r.Name, &r.PlaytimeTotal, &r.PlaytimeRecent, &r.LastPolled); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// exportHistory streams a CSV of all play history for a user (premium only).
// Query param: user_id
func exportHistory(c *routing.Context) error {
	userID := string(c.QueryArgs().Peek("user_id"))
	if userID == "" {
		c.SetStatusCode(400)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"user_id is required"}`)
		return nil
	}

	ok, err := premiums.checkPremium(c.RequestCtx, userID)
	if err != nil || !ok {
		c.SetStatusCode(402)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"premium subscription required","upgrade_url":"/api/stripe/checkout"}`)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err2 := findExportHistory(ctx, userID)
	if err2 != nil {
		c.SetStatusCode(500)
		c.WriteString("failed to fetch history")
		return nil
	}

	c.SetContentType("text/csv")
	c.Response.Header.Set("Content-Disposition", `attachment; filename="gameroom_history.csv"`)

	w := csv.NewWriter(c)
	_ = w.Write([]string{"app_id", "name", "playtime_total_minutes", "playtime_recent_minutes", "last_polled"})

	for _, row := range rows {
		lastPolledStr := ""
		if !row.LastPolled.IsZero() {
			lastPolledStr = row.LastPolled.UTC().Format(time.RFC3339)
		}
		_ = w.Write([]string{
			strconv.Itoa(row.AppID),
			row.Name,
			strconv.Itoa(row.PlaytimeTotal),
			strconv.Itoa(row.PlaytimeRecent),
			lastPolledStr,
		})
	}
	w.Flush()
	return nil
}

// getGameDeals returns stored deal offers for a game, with affiliate-tagged URLs.
// Route param: title
func getGameDeals(c *routing.Context) error {
	title := c.Param("title")

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	var game Game
	var pcJSON, xboxJSON, psJSON, switchJSON []byte
	err := db.QueryRow(ctx, `
		SELECT title, img, pc_offers, xbox_offers, playstation_offers, switch_offers
		FROM games
		WHERE title = $1
	`, title).Scan(&game.Title, &game.Img, &pcJSON, &xboxJSON, &psJSON, &switchJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		c.SetStatusCode(404)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"game not found"}`)
		return nil
	}
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"database error"}`)
		return nil
	}

	json.Unmarshal(pcJSON, &game.PcOffers)
	json.Unmarshal(xboxJSON, &game.XboxOffers)
	json.Unmarshal(psJSON, &game.PlaystationOffers)
	json.Unmarshal(switchJSON, &game.SwitchOffers)

	tagOffers := func(offers []Offer) []map[string]interface{} {
		result := make([]map[string]interface{}, 0, len(offers))
		for _, o := range offers {
			result = append(result, map[string]interface{}{
				"vendor":      o.Vendor,
				"platform":    o.Platform,
				"digital":     o.Digital,
				"url":         o.Url,
				"deal_url":    tagURL(o.Url, o.Vendor),
				"internal_id": o.InternalId,
			})
		}
		return result
	}

	c.SetContentType("application/json")
	out, _ := json.Marshal(map[string]interface{}{
		"title":             game.Title,
		"pc_deals":          tagOffers(game.PcOffers),
		"xbox_deals":        tagOffers(game.XboxOffers),
		"playstation_deals": tagOffers(game.PlaystationOffers),
		"switch_deals":      tagOffers(game.SwitchOffers),
		"find_more_deals":   itadSearchURL(game.Title),
	})
	c.Write(out)
	return nil
}

// getPublicProfile returns a user's year-in-games summary and activity heatmap.
// Route param: user_id. No authentication required — the profile is public.
func getPublicProfile(c *routing.Context) error {
	userID := c.Param("user_id")
	now := time.Now().UTC()
	year := now.Year()
	startOfYear := fmt.Sprintf("%d-01-01", year)
	yearAgo := now.AddDate(-1, 0, 0).Format("2006-01-02")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Top games this calendar year, ranked by total minutes.
	topRows, err := db.Query(ctx, `
		SELECT app_id, MAX(name) AS name, SUM(minutes) AS minutes
		FROM play_activity
		WHERE user_id = $1 AND date >= $2
		GROUP BY app_id
		ORDER BY minutes DESC
		LIMIT 10
	`, userID, startOfYear)
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to fetch games"}`)
		return nil
	}
	defer topRows.Close()

	topGames := []PublicGame{}
	for topRows.Next() {
		var pg PublicGame
		if err := topRows.Scan(&pg.AppID, &pg.Name, &pg.Minutes); err != nil {
			c.SetStatusCode(500)
			c.SetContentType("application/json")
			c.WriteString(`{"error":"failed to fetch games"}`)
			return nil
		}
		topGames = append(topGames, pg)
	}
	if err := topRows.Err(); err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to fetch games"}`)
		return nil
	}

	topRows.Close()

	// Totals across all games this year.
	var totalMinutes, totalGames int
	if err := db.QueryRow(ctx, `
		SELECT COALESCE(SUM(minutes), 0), COUNT(DISTINCT app_id)
		FROM play_activity
		WHERE user_id = $1 AND date >= $2
	`, userID, startOfYear).Scan(&totalMinutes, &totalGames); err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to fetch totals"}`)
		return nil
	}

	// Daily activity totals for the past 52 weeks (heatmap).
	heatRows, err := db.Query(ctx, `
		SELECT date, SUM(minutes) AS minutes
		FROM play_activity
		WHERE user_id = $1 AND date >= $2
		GROUP BY date
		ORDER BY date ASC
	`, userID, yearAgo)
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to fetch heatmap"}`)
		return nil
	}
	defer heatRows.Close()

	heatmap := []HeatmapDay{}
	for heatRows.Next() {
		var hd HeatmapDay
		if err := heatRows.Scan(&hd.Date, &hd.Minutes); err != nil {
			c.SetStatusCode(500)
			c.SetContentType("application/json")
			c.WriteString(`{"error":"failed to fetch heatmap"}`)
			return nil
		}
		heatmap = append(heatmap, hd)
	}
	if err := heatRows.Err(); err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to fetch heatmap"}`)
		return nil
	}

	c.SetContentType("application/json")
	c.Response.Header.Set("Access-Control-Allow-Origin", "*")
	out, _ := json.Marshal(map[string]interface{}{
		"user_id":       userID,
		"year":          year,
		"total_minutes": totalMinutes,
		"total_games":   totalGames,
		"top_games":     topGames,
		"heatmap":       heatmap,
	})
	c.Write(out)
	return nil
}

// serveFrontend serves the built React app, falling back to index.html for SPA routing.
func serveFrontend(ctx *fasthttp.RequestCtx) {
	filePath := "build" + string(ctx.Path())
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		fasthttp.ServeFile(ctx, filePath)
		return
	}
	if _, err := os.Stat("build/index.html"); os.IsNotExist(err) {
		ctx.SetStatusCode(503)
		ctx.SetBodyString("Frontend not built. Run: npm run build")
		return
	}
	fasthttp.ServeFile(ctx, "build/index.html")
}

//
// MAIN
//

func pollUserPlayHistory(steamID, userID, key string) {
	query := url.Values{}
	query.Set("key", key)
	query.Set("steamid", steamID)
	query.Set("count", "10")
	query.Set("format", "json")
	resp, err := steamGet("/IPlayerService/GetRecentlyPlayedGames/v0001/?" + query.Encode())
	if err != nil {
		log.Printf("play-history poller: Steam API error for %s: %v", steamID, err)
		return
	}

	v, err := fastjson.ParseBytes(resp)
	if err != nil {
		log.Printf("play-history poller: parse error for %s: %v", steamID, err)
		return
	}

	games := v.GetArray("response", "games")
	if len(games) == 0 {
		return
	}

	now := time.Now().UTC()

	for _, g := range games {
		appID := g.GetInt("appid")
		name := string(g.GetStringBytes("name"))
		playtimeTotal := g.GetInt("playtime_forever")
		playtimeRecent := g.GetInt("playtime_2weeks")

		// Record daily playtime delta for the public-profile heatmap.
		findCtx, findCancel := context.WithTimeout(context.Background(), dbTimeout)
		var oldPT int
		findErr := db.QueryRow(findCtx, `
			SELECT playtime_total FROM play_history WHERE user_id = $1 AND app_id = $2
		`, userID, appID).Scan(&oldPT)
		findCancel()
		if findErr == nil {
			if delta := playtimeTotal - oldPT; delta > 0 {
				actCtx, actCancel := context.WithTimeout(context.Background(), dbTimeout)
				_, actErr := db.Exec(actCtx, `
					INSERT INTO play_activity (user_id, app_id, name, date, minutes)
					VALUES ($1, $2, $3, $4, $5)
					ON CONFLICT (user_id, app_id, date) DO UPDATE SET
						minutes = play_activity.minutes + $5,
						name    = $3
				`, userID, appID, name, now.Format("2006-01-02"), delta)
				actCancel()
				if actErr != nil {
					log.Printf("play-history poller: activity upsert error for user %s app %d: %v", userID, appID, actErr)
				}
			}
		}

		historyCtx, historyCancel := context.WithTimeout(context.Background(), dbTimeout)
		_, err := db.Exec(historyCtx, `
			INSERT INTO play_history (user_id, steam_id, app_id, name, playtime_total, playtime_recent, last_polled)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (user_id, app_id) DO UPDATE SET
				steam_id        = $2,
				name            = $4,
				playtime_total  = $5,
				playtime_recent = $6,
				last_polled     = $7
		`, userID, steamID, appID, name, playtimeTotal, playtimeRecent, now)
		historyCancel()
		if err != nil {
			log.Printf("play-history poller: upsert error for user %s app %d: %v", userID, appID, err)
		}
	}
}

func pollPlayHistory() {
	key := steamAPIKey()
	if key == "" {
		log.Println("play-history poller: STEAM_API_KEY not set, skipping")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `SELECT user_id, steam_id FROM steam_links`)
	if err != nil {
		log.Println("play-history poller: failed to fetch steam_links:", err)
		return
	}
	defer rows.Close()

	var links []SteamLink
	for rows.Next() {
		var l SteamLink
		if err := rows.Scan(&l.UserID, &l.SteamID); err != nil {
			log.Println("play-history poller: failed to scan steam_links:", err)
			return
		}
		links = append(links, l)
	}
	if err := rows.Err(); err != nil {
		log.Println("play-history poller: failed to iterate steam_links:", err)
		return
	}
	rows.Close()

	for _, link := range links {
		pollUserPlayHistory(link.SteamID, link.UserID, key)
	}
}

func startPlayHistoryPoller() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	pollPlayHistory()
	for range ticker.C {
		pollPlayHistory()
	}
}

func main() {
	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("JWT_SECRET must be set")
	}

	ctx := context.Background()
	initDB(ctx)
	migrateDB(ctx)

	router := routing.New()

	go scrapeSteam(5000)
	go startPlayHistoryPoller()

	//
	// API
	//

	router.Get("/api", func(c *routing.Context) error {
		fmt.Fprintf(c, "gameroom API v1.0.0")
		return nil
	})

	// Game Info Route
	router.Get("/game/<title>", func(c *routing.Context) error {
		fmt.Fprintf(c, "gameroom API v1.0.0")
		return nil
	})

	// Search
	router.Get("/search/<term>", func(c *routing.Context) error {
		fmt.Fprintf(c, "gameroom API v1.0.0")
		return nil
	})

	// User accounts
	router.Post("/auth/register", func(c *routing.Context) error { return registerUser(c) })
	router.Post("/auth/login", func(c *routing.Context) error { return loginUser(c) })
	router.Get("/auth/me", func(c *routing.Context) error {
		if !authMiddleware(c) {
			return nil
		}
		return getMe(c)
	})

	router.Post("/s/link", func(c *routing.Context) error {
		if !authMiddleware(c) {
			return nil
		}
		return linkAccount(c)
	})

	router.Get("/s/status", func(c *routing.Context) error {
		return getCurrentGame(c)
	})

	router.Get("/s/recent", func(c *routing.Context) error {
		return getRecentGames(c)
	})

	// Premium subscription
	router.Post("/api/stripe/checkout", func(c *routing.Context) error {
		return createCheckoutSession(c)
	})
	router.Post("/api/stripe/webhook", func(c *routing.Context) error {
		return handleStripeWebhook(c)
	})

	// Premium entitlement check (authenticated; used by /premium/success to verify activation)
	router.Get("/api/premium/status", func(c *routing.Context) error {
		if !authMiddleware(c) {
			return nil
		}
		return getPremiumStatus(c)
	})

	// Premium-gated endpoints
	router.Get("/u/stats", func(c *routing.Context) error {
		return getDeepStats(c)
	})
	router.Get("/u/export", func(c *routing.Context) error {
		return exportHistory(c)
	})

	// Xbox Live OAuth linking
	router.Get("/x/auth", func(c *routing.Context) error {
		if !authMiddleware(c) {
			return nil
		}
		return xboxAuth(c)
	})
	router.Get("/x/callback", func(c *routing.Context) error { return xboxCallback(c) })

	// PlayStation Network OAuth linking
	router.Get("/p/auth", func(c *routing.Context) error {
		if !authMiddleware(c) {
			return nil
		}
		return psnAuth(c)
	})
	router.Get("/p/callback", func(c *routing.Context) error { return psnCallback(c) })

	// Epic Games OAuth linking
	router.Get("/e/auth", func(c *routing.Context) error {
		if !authMiddleware(c) {
			return nil
		}
		return epicAuth(c)
	})
	router.Get("/e/callback", func(c *routing.Context) error { return epicCallback(c) })

	// Game deals with affiliate links
	router.Get("/game/<title>/deals", func(c *routing.Context) error {
		return getGameDeals(c)
	})

	// Public profile: year-in-games + activity heatmap
	router.Get("/u/<user_id>/profile", func(c *routing.Context) error {
		return getPublicProfile(c)
	})

	// Retention metrics (requires Authorization: Bearer <ADMIN_KEY>; query parameters are rejected.)
	router.Get("/api/admin/retention", func(c *routing.Context) error {
		return getRetentionStats(c)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	// API routes take precedence; unmatched paths fall through to the React SPA.
	router.NotFound(func(c *routing.Context) error {
		serveFrontend(c.RequestCtx)
		return nil
	})
	panic(fasthttp.ListenAndServe(":"+port, router.HandleRequest))
}
