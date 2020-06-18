package main

import (
	"fmt"
	"context"
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

	router.Get("/s/status/<id>", func(c *routing.Context) error {
		ctx, cancel := chromedp.NewContext(context.Background())
		fmt.Println("context")
		defer cancel()
		game := ""
		err := chromedp.Run(ctx,
			chromedp.Navigate("https://steamcommunity.com/id/" + c.Param("id")),
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