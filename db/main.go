package main

import (
	"fmt"
	"context"
	"time"
	"log"
	"strconv"
	//"encoding/json"
	"github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
	"github.com/chromedp/chromedp"
	"go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

var mongoClient *mongo.Client

type Offer struct {
	Vendor string
	Digital bool
	Platform string
	Url string
	InternalId string
}

type Game struct {
	Title string
	Img string
	Dev string
	Pub string
	RDate string
	Metacritic string
	PcOffers []Offer
	XboxOffers []Offer
	PlaystationOffers []Offer
	SwitchOffers []Offer
	Dlc []DLC
	Platforms []string
}

type DLC struct {
	game Game
	title string
	img string
	rDate string
	pcOffers []Offer
	xboxOffers []Offer
	playstationOffers []Offer
	switchOffers []Offer
}

type SteamResult struct {
	Name string
}

// func createOffer(v string, d bool, p string, u string, i string) {

// }

// func createDLC(g Game, t string, i string, r string, offer Offer) {

// }

// func createGame(t string, i string, d []string, p string[], r string, m int, offer Offer) {

// }

//
// Steam Pricing
//

func readSteamDLC(i int, g Game) string {
	var body []byte
	id := strconv.Itoa(i)

	status, resp, err := fasthttp.Get(body, "https://store.steampowered.com/api/appdetails?appids=" + id)
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

	status, resp, err := fasthttp.Get(body, "https://store.steampowered.com/api/appdetails?appids=" + id)
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
					Title: title,
					Img: fastjson.GetString(resp, id, "data", "header_image"),
					Dev: fastjson.GetString(resp, id, "data", "developers", "0"),
					Pub: fastjson.GetString(resp, id, "data", "publishers", "0"),
					RDate: fastjson.GetString(resp, id, "data", "release_date", "date"),
					Metacritic: fastjson.GetString(resp, id, "data", "metacritic", "url"),
					PcOffers: []Offer{},
					XboxOffers: []Offer{},
					PlaystationOffers: []Offer{},
					SwitchOffers: []Offer{},
					Dlc: []DLC{},
					Platforms: []string{},
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
					Vendor: "steam",
					Digital: true,
					Platform: "pc",
					Url: "https://store.steampowered.com/app/" + appid + "/",
					InternalId: appid,
				}
				fmt.Println(newOffer)
				newGame.PcOffers = append(newGame.PcOffers, newOffer)

				fmt.Println(newGame)

				// Add to DB
				insertResult, err2 := collection.InsertOne(context.TODO(), newGame)
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
	for i := 0;  i < 1000000; i += 10 {
		readSteamGame(i)
		time.Sleep(latency * time.Millisecond)
	}
}

//
// Steam Accounts
// 

func getCurrentGame(c *routing.Context) error {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		// Set the headless flag to false to display the browser window
		chromedp.Flag("headless", false),
		)
		
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel = chromedp.NewContext(ctx)

	fmt.Println("context")
	defer cancel()
	var game string
	err := chromedp.Run(ctx,
		//chromedp.EmulateViewport(1200, 2000),
		chromedp.Navigate("https://steamcommunity.com/id/fybermain/"),
		chromedp.Sleep(5*time.Second),
		chromedp.Evaluate(`document.getElementsByClassName('profile_in_game_name')[0].innerHTML`, &game),
	)
	fmt.Println(game)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Fprintf(c, game)
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
		fmt.Println("error in ListenAndServe: %s", err)
	}
}

//
// MAIN
//

func main() {
	fmt.Println("Running...")
	router := routing.New()

	//
	// MongoDB
	//

	// Set client options
	clientOptions := options.Client().ApplyURI("mongodb+srv://admin:CzP41byffHs9CIVd@gamium1-7iud8.mongodb.net/Gamium>?retryWrites=true&w=majority")

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

	//collection := client.Database("test").Collection("trainers")

	//
	// API
	//
	
	router.Get("/", func(c *routing.Context) error {
		fmt.Println("Hello, world!")
		fmt.Fprintf(c, "Gamium API v1.0.0")
		return nil
	})

	// Game Info Route
	router.Get("/game/<title>", func(c *routing.Context) error {
		fmt.Println("Hello, world!")
		fmt.Fprintf(c, "Gamium API v1.0.0")
		return nil
	})

	// Search
	router.Get("/search/<term>", func(c *routing.Context) error {
		fmt.Println("Hello, world!")
		fmt.Fprintf(c, "Gamium API v1.0.0")
		return nil
	})



	router.Post("/s/link", func(c *routing.Context) error {
		// Authenticate User
		// Verify Steam Credentials
		// Load Library Data
		// Save
		return nil
	})

	router.Get("/s/status", func(c *routing.Context) error {
		// Check if Online
		return getCurrentGame(c)
	})

	//
	// Webapp
	//
	
	go serveWebapp()

	panic(fasthttp.ListenAndServe(":8080", router.HandleRequest))
}

