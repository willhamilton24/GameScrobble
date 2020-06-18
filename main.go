package main

import (
	"fmt"
	"context"
	"time"
	"github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
	"github.com/chromedp/chromedp"
)

func main() {
	router := routing.New()
	
	router.Get("/", func(c *routing.Context) error {
		fmt.Println("Hello, world!")
		fmt.Fprintf(c, "Hello, world!")
		return nil
	})

	router.Get("/s/status", func(c *routing.Context) error {
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
	})
	
	panic(fasthttp.ListenAndServe(":8080", router.HandleRequest))
}