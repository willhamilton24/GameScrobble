package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	routing "github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"
)

var mongoClient *mongo.Client

const steamRequestTimeout = 5 * time.Second
const mongoUpdateTimeout = 5 * time.Second

var steamHTTPGet = fasthttp.GetTimeout

// Only authMiddleware (auth.go) may set this context key.
type authenticatedUserIDKey struct{}

type steamLinkUpdater interface {
	UpdateOne(context.Context, interface{}, interface{}, ...*options.UpdateOptions) (*mongo.UpdateResult, error)
}

func saveSteamLink(parent context.Context, collection steamLinkUpdater, link SteamLink) error {
	ctx, cancel := context.WithTimeout(parent, mongoUpdateTimeout)
	defer cancel()
	_, err := collection.UpdateOne(ctx,
		bson.D{{Key: "user_id", Value: link.UserID}},
		bson.D{{Key: "$set", Value: link}}, options.Update().SetUpsert(true))
	return err
}

type Offer struct {
	Vendor     string
	Digital    bool
	Platform   string
	Url        string
	InternalId string
}

type Game struct {
	Title             string
	Img               string
	Dev               string
	Pub               string
	RDate             string
	Metacritic        string
	PcOffers          []Offer
	XboxOffers        []Offer
	PlaystationOffers []Offer
	SwitchOffers      []Offer
	Dlc               []DLC
	Platforms         []string
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
	UserID            string `bson:"user_id" json:"user_id"`
	SteamID           string `bson:"steam_id" json:"steam_id"`
	OwnershipVerified bool   `bson:"ownership_verified" json:"ownership_verified"`
}

// PlayActivity records a daily playtime delta used to build the public profile heatmap.
type PlayActivity struct {
	UserID  string `bson:"user_id"`
	AppID   int    `bson:"app_id"`
	Name    string `bson:"name"`
	Date    string `bson:"date"` // YYYY-MM-DD
	Minutes int    `bson:"minutes"`
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
		// Check Existance
		filter := bson.D{{"title", title}}
		var result Game
		collection := *mongoClient.Database("Gamium").Collection("games")

		if err = collection.FindOne(context.TODO(), filter).Decode(&result); err != nil {
			errStr := err.Error()
			if errStr == "mongo: no documents in result" {
				fmt.Println("NOT SAVED")

				// XX Shelved XX
				// Check for DLC Array
				// Iterate Through DLC
				//dlcIds := getDLCArray(resp, id)
				//fmt.Println(strconv.Itoa(dlcIds[0]))

				// Make Game Struct
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
					Dlc:               []DLC{},
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

				// Offer
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

				// Add to DB
				writeCtx, cancel := context.WithTimeout(context.Background(), mongoUpdateTimeout)
				insertResult, err2 := collection.InsertOne(writeCtx, newGame)
				cancel()
				if err2 != nil {
					log.Fatal(err)
				}

				fmt.Println("Inserted a single document: ", insertResult.InsertedID)
			} else {
				fmt.Println(err.Error())
			}
		} else {
			fmt.Println(result)
			// Offer
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

	// Upsert the link in MongoDB
	collection := mongoClient.Database("Gamium").Collection("steam_links")
	if err = saveSteamLink(c.RequestCtx, collection, link); err != nil {
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

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "user_id", Value: userID}}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$app_id"},
			{Key: "name", Value: bson.D{{Key: "$last", Value: "$name"}}},
			{Key: "playtime_total", Value: bson.D{{Key: "$max", Value: "$playtime_total"}}},
			{Key: "playtime_recent", Value: bson.D{{Key: "$last", Value: "$playtime_recent"}}},
			{Key: "last_polled", Value: bson.D{{Key: "$max", Value: "$last_polled"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "playtime_total", Value: -1}}}},
	}

	cursor, err2 := mongoClient.Database("Gamium").Collection("scrobbles").Aggregate(ctx, pipeline)
	if err2 != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to fetch stats"}`)
		return nil
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err2 = cursor.All(ctx, &results); err2 != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to decode stats"}`)
		return nil
	}
	if results == nil {
		results = []bson.M{}
	}

	c.SetContentType("application/json")
	out, _ := json.Marshal(map[string]interface{}{"stats": results})
	c.Write(out)
	return nil
}

// findExportHistory can be replaced with an in-memory cursor in handler tests.
var findExportHistory = func(ctx context.Context, userID string) (*mongo.Cursor, error) {
	return mongoClient.Database("Gamium").Collection("scrobbles").
		Find(ctx, bson.D{{Key: "user_id", Value: userID}})
}

// exportHistory streams a CSV of all scrobbles for a user (premium only).
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

	cursor, err2 := findExportHistory(ctx, userID)
	if err2 != nil {
		c.SetStatusCode(500)
		c.WriteString("failed to fetch history")
		return nil
	}
	defer cursor.Close(ctx)

	c.SetContentType("text/csv")
	c.Response.Header.Set("Content-Disposition", `attachment; filename="gamescrobble_history.csv"`)

	w := csv.NewWriter(c)
	_ = w.Write([]string{"app_id", "name", "playtime_total_minutes", "playtime_recent_minutes", "last_polled"})

	for cursor.Next(ctx) {
		var row bson.M
		if err3 := cursor.Decode(&row); err3 != nil {
			continue
		}
		appID := fmt.Sprintf("%v", row["app_id"])
		name, _ := row["name"].(string)
		playtimeTotal := fmt.Sprintf("%v", row["playtime_total"])
		playtimeRecent := fmt.Sprintf("%v", row["playtime_recent"])
		lastPolled := ""
		if t, ok2 := row["last_polled"].(time.Time); ok2 {
			lastPolled = t.UTC().Format(time.RFC3339)
		}
		_ = w.Write([]string{appID, name, playtimeTotal, playtimeRecent, lastPolled})
	}
	w.Flush()
	return nil
}

// getGameDeals returns stored deal offers for a game, with affiliate-tagged URLs.
// Route param: title
func getGameDeals(c *routing.Context) error {
	title := c.Param("title")

	ctx, cancel := context.WithTimeout(context.Background(), mongoUpdateTimeout)
	defer cancel()

	var game Game
	err := mongoClient.Database("Gamium").Collection("games").
		FindOne(ctx, bson.D{{Key: "title", Value: title}}).Decode(&game)
	if err != nil {
		c.SetStatusCode(404)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"game not found"}`)
		return nil
	}

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

	actColl := mongoClient.Database("Gamium").Collection("play_activity")

	// Top games this calendar year, ranked by total minutes.
	topPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "user_id", Value: userID},
			{Key: "date", Value: bson.D{{Key: "$gte", Value: startOfYear}}},
		}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$app_id"},
			{Key: "name", Value: bson.D{{Key: "$last", Value: "$name"}}},
			{Key: "minutes", Value: bson.D{{Key: "$sum", Value: "$minutes"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "minutes", Value: -1}}}},
		{{Key: "$limit", Value: 10}},
	}
	cursor, err := actColl.Aggregate(ctx, topPipeline)
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to fetch games"}`)
		return nil
	}
	defer cursor.Close(ctx)
	var rawGames []bson.M
	if err = cursor.All(ctx, &rawGames); err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to decode games"}`)
		return nil
	}

	topGames := make([]PublicGame, 0, len(rawGames))
	for _, g := range rawGames {
		pg := PublicGame{Name: fmt.Sprintf("%v", g["name"])}
		switch v := g["_id"].(type) {
		case int32:
			pg.AppID = int(v)
		case int64:
			pg.AppID = int(v)
		case int:
			pg.AppID = v
		}
		switch v := g["minutes"].(type) {
		case int32:
			pg.Minutes = int(v)
		case int64:
			pg.Minutes = int(v)
		case int:
			pg.Minutes = v
		}
		topGames = append(topGames, pg)
	}

	// Totals across all games this year (not just the top 10).
	totalsPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "user_id", Value: userID},
			{Key: "date", Value: bson.D{{Key: "$gte", Value: startOfYear}}},
		}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$app_id"},
			{Key: "minutes", Value: bson.D{{Key: "$sum", Value: "$minutes"}}},
		}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "total_minutes", Value: bson.D{{Key: "$sum", Value: "$minutes"}}},
			{Key: "total_games", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	}
	totalsCursor, err := actColl.Aggregate(ctx, totalsPipeline)
	if err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to fetch totals"}`)
		return nil
	}
	defer totalsCursor.Close(ctx)
	var rawTotals []bson.M
	if err = totalsCursor.All(ctx, &rawTotals); err != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to decode totals"}`)
		return nil
	}

	totalMinutes := 0
	totalGames := 0
	if len(rawTotals) > 0 {
		switch v := rawTotals[0]["total_minutes"].(type) {
		case int32:
			totalMinutes = int(v)
		case int64:
			totalMinutes = int(v)
		case int:
			totalMinutes = v
		}
		switch v := rawTotals[0]["total_games"].(type) {
		case int32:
			totalGames = int(v)
		case int64:
			totalGames = int(v)
		case int:
			totalGames = v
		}
	}

	// Daily activity totals for the past 52 weeks (heatmap).
	heatPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "user_id", Value: userID},
			{Key: "date", Value: bson.D{{Key: "$gte", Value: yearAgo}}},
		}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$date"},
			{Key: "minutes", Value: bson.D{{Key: "$sum", Value: "$minutes"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}
	cursor2, err2 := actColl.Aggregate(ctx, heatPipeline)
	if err2 != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to fetch heatmap"}`)
		return nil
	}
	defer cursor2.Close(ctx)
	var rawHeat []bson.M
	if err2 = cursor2.All(ctx, &rawHeat); err2 != nil {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to decode heatmap"}`)
		return nil
	}

	heatmap := make([]HeatmapDay, 0, len(rawHeat))
	for _, h := range rawHeat {
		hd := HeatmapDay{Date: fmt.Sprintf("%v", h["_id"])}
		switch v := h["minutes"].(type) {
		case int32:
			hd.Minutes = int(v)
		case int64:
			hd.Minutes = int(v)
		case int:
			hd.Minutes = v
		}
		heatmap = append(heatmap, hd)
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

func pollUserScrobbles(steamID, userID, key string) {
	query := url.Values{}
	query.Set("key", key)
	query.Set("steamid", steamID)
	query.Set("count", "10")
	query.Set("format", "json")
	resp, err := steamGet("/IPlayerService/GetRecentlyPlayedGames/v0001/?" + query.Encode())
	if err != nil {
		log.Printf("scrobble poller: Steam API error for %s: %v", steamID, err)
		return
	}

	v, err := fastjson.ParseBytes(resp)
	if err != nil {
		log.Printf("scrobble poller: parse error for %s: %v", steamID, err)
		return
	}

	games := v.GetArray("response", "games")
	if len(games) == 0 {
		return
	}

	coll := mongoClient.Database("Gamium").Collection("scrobbles")
	now := time.Now().UTC()

	for _, g := range games {
		appID := g.GetInt("appid")
		name := string(g.GetStringBytes("name"))
		playtimeTotal := g.GetInt("playtime_forever")
		playtimeRecent := g.GetInt("playtime_2weeks")

		// Record daily playtime delta for the public-profile heatmap.
		findCtx, findCancel := context.WithTimeout(context.Background(), mongoUpdateTimeout)
		var existing bson.M
		findErr := coll.FindOne(findCtx, bson.D{
			{Key: "user_id", Value: userID},
			{Key: "app_id", Value: appID},
		}).Decode(&existing)
		findCancel()
		if findErr == nil {
			oldPT := 0
			switch v := existing["playtime_total"].(type) {
			case int32:
				oldPT = int(v)
			case int64:
				oldPT = int(v)
			case int:
				oldPT = v
			}
			if delta := playtimeTotal - oldPT; delta > 0 {
				actColl := mongoClient.Database("Gamium").Collection("play_activity")
				actCtx, actCancel := context.WithTimeout(context.Background(), mongoUpdateTimeout)
				_, actErr := actColl.UpdateOne(actCtx,
					bson.D{
						{Key: "user_id", Value: userID},
						{Key: "app_id", Value: appID},
						{Key: "date", Value: now.Format("2006-01-02")},
					},
					bson.D{
						{Key: "$inc", Value: bson.D{{Key: "minutes", Value: delta}}},
						{Key: "$set", Value: bson.D{{Key: "name", Value: name}}},
					},
					options.Update().SetUpsert(true),
				)
				actCancel()
				if actErr != nil {
					log.Printf("scrobble poller: activity upsert error for user %s app %d: %v", userID, appID, actErr)
				}
			}
		}

		scrobble := bson.D{
			{Key: "user_id", Value: userID},
			{Key: "steam_id", Value: steamID},
			{Key: "app_id", Value: appID},
			{Key: "name", Value: name},
			{Key: "playtime_total", Value: playtimeTotal},
			{Key: "playtime_recent", Value: playtimeRecent},
			{Key: "last_polled", Value: now},
		}

		ctx, cancel := context.WithTimeout(context.Background(), mongoUpdateTimeout)
		_, err := coll.UpdateOne(ctx,
			bson.D{{Key: "user_id", Value: userID}, {Key: "app_id", Value: appID}},
			bson.D{{Key: "$set", Value: scrobble}},
			options.Update().SetUpsert(true),
		)
		cancel()
		if err != nil {
			log.Printf("scrobble poller: mongo upsert error for user %s app %d: %v", userID, appID, err)
		}
	}
}

func pollScrobbles() {
	key := steamAPIKey()
	if key == "" {
		log.Println("scrobble poller: STEAM_API_KEY not set, skipping")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cursor, err := mongoClient.Database("Gamium").Collection("steam_links").Find(ctx, bson.D{})
	if err != nil {
		log.Println("scrobble poller: failed to fetch steam_links:", err)
		return
	}
	defer cursor.Close(ctx)

	var links []SteamLink
	if err = cursor.All(ctx, &links); err != nil {
		log.Println("scrobble poller: failed to decode steam_links:", err)
		return
	}

	for _, link := range links {
		pollUserScrobbles(link.SteamID, link.UserID, key)
	}
}

func startScrobblePoller() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	pollScrobbles()
	for range ticker.C {
		pollScrobbles()
	}
}

func mongoURI() string {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		log.Fatal("MONGODB_URI environment variable is required")
	}
	return uri
}

func main() {
	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("JWT_SECRET must be set")
	}

	router := routing.New()

	//
	// MongoDB
	//

	clientOptions := options.Client().ApplyURI(mongoURI())

	// Connect to MongoDB
	client, err := mongo.Connect(context.TODO(), clientOptions)

	if err != nil {
		log.Fatal(err)
	}

	// Check the connection
	err = client.Ping(context.TODO(), nil)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected to MongoDB!")

	mongoClient = client

	// Enforce email uniqueness at the database level so concurrent
	// registrations cannot create duplicate accounts.
	idxCtx, idxCancel := context.WithTimeout(context.Background(), mongoUpdateTimeout)
	_, err = client.Database("Gamium").Collection("users").Indexes().CreateOne(idxCtx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	idxCancel()
	if err != nil {
		log.Fatal("failed to create unique index on users.email: ", err)
	}

	//
	// Data
	//
	go scrapeSteam(5000)
	go startScrobblePoller()

	//collection := client.Database("test").Collection("trainers")

	//
	// API
	//

	router.Get("/api", func(c *routing.Context) error {
		fmt.Fprintf(c, "Gamium API v1.0.0")
		return nil
	})

	// Game Info Route
	router.Get("/game/<title>", func(c *routing.Context) error {
		fmt.Fprintf(c, "Gamium API v1.0.0")
		return nil
	})

	// Search
	router.Get("/search/<term>", func(c *routing.Context) error {
		fmt.Fprintf(c, "Gamium API v1.0.0")
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
