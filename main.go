package main

import (
	"fmt"
	"context"
	"time"
	"log"
	"github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
	"github.com/chromedp/chromedp"
	//"go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

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

type Offer struct {
	vendor string
	digital bool
	platform string
	url string
}

type Game struct {
	title string
	devs []string
	pubs []string
	rDate string
	metacritic int
	pcOffers []string
	xboxOffers []string
	playstationOffers []string
}

func main() {
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

	//collection := client.Database("test").Collection("trainers")

	//
	// API
	//
	
	router.Get("/", func(c *routing.Context) error {
		fmt.Println("Hello, world!")
		fmt.Fprintf(c, "Hello, world!")
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

