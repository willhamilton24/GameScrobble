package main

import (
	"context"
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

// Only trusted authentication middleware may set this context key. No current
// route sets it, so linking remains disabled until authentication is implemented.
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
// Serve Webapp
//

func serveWebapp() {
	fs := &fasthttp.FS{
		// Path to directory to serve.
		Root: "index.html",
		// Generate index pages if client requests directory contents.
		GenerateIndexPages: true,
		// Enable transparent compression to save network traffic.
		Compress: true,
	}

	// Create request handler for serving static files.
	h := fs.NewRequestHandler()

	// Start the server.
	if err := fasthttp.ListenAndServe(":3000", h); err != nil {
		fmt.Println("error in ListenAndServe:", err)
	}
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

	//
	// Data
	//
	go scrapeSteam(5000)
	go startScrobblePoller()

	//collection := client.Database("test").Collection("trainers")

	//
	// API
	//

	router.Get("/", func(c *routing.Context) error {
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

	router.Post("/s/link", func(c *routing.Context) error {
		return linkAccount(c)
	})

	router.Get("/s/status", func(c *routing.Context) error {
		return getCurrentGame(c)
	})

	router.Get("/s/recent", func(c *routing.Context) error {
		return getRecentGames(c)
	})

	//
	// Webapp
	//

	go serveWebapp()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	panic(fasthttp.ListenAndServe(":"+port, router.HandleRequest))
}
