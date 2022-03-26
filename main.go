package main

import (
	"fmt"
	"github.com/adshao/go-binance/v2"
	"github.com/cenkalti/backoff/v4"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const defaultDuration = time.Second * 5

type Pair struct {
	Name   string
	Symbol string
}

var pairs = []Pair{
	{Name: "ETH", Symbol: "ETHUSDT"},
	{Name: "BTC", Symbol: "BTCUSDT"},
}

func nexPairGen(pairs []Pair, duration time.Duration) func() Pair {
	if len(pairs) == 0 {
		panic("at least one pair must be provided")
	}

	var (
		start time.Time
		pos   int
	)

	return func() Pair {
		if start.IsZero() {
			start = time.Now()

			return pairs[pos]
		}

		if time.Since(start) >= duration {
			pos++
			start = time.Now()
		}

		if pos >= len(pairs) {
			pos = 0
		}

		return pairs[pos]
	}
}

type Client struct {
	filePath string
	nextPair func() Pair
}

func priceStr(symbol string, val float64) string {
	p := message.NewPrinter(language.English)

	switch symbol {
	case "ETHUSDT", "BTCUSDT":
		return p.Sprintf("%d", int64(val))
	default:
		return p.Sprintf("%.2f", val)
	}
}

func (*Client) errHandler(err error) { log.Println(err) }

func (c *Client) eventHandler(events binance.WsAllMiniMarketsStatEvent) {
	for i := range events {
		pair := c.nextPair()

		if events[i].Symbol == pair.Symbol {
			lastPrice, err := strconv.ParseFloat(events[i].LastPrice, 64)
			if err != nil {
				log.Println("parse last price:", err)

				continue
			}

			openPrice, err := strconv.ParseFloat(events[i].OpenPrice, 64)
			if err != nil {
				log.Println("parse open price:", err)

				continue
			}

			var direction = "↓"
			var color = "#FF0000"
			var percent = (math.Round((((openPrice-lastPrice)/openPrice)*100)*100) / 100) * -1

			if openPrice < lastPrice {
				direction = "↑"
				color = "#008000"
			}

			line := fmt.Sprintf(
				"<span foreground='#FFFFFF'>%s $%s</span> <span foreground='%s'>%s %.2f%%</span>",
				pair.Name, priceStr(pair.Symbol, lastPrice), color, direction, percent,
			)

			os.WriteFile(c.filePath, []byte(line), 0644)
		}
	}
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Println("user home dir:", err)
	}

	var client = Client{
		filePath: home + string(os.PathSeparator) + ".binance",
		nextPair: nexPairGen(pairs, defaultDuration),
	}

	var done, stop chan struct{}

	err = backoff.Retry(func() error {
		done, stop, err = binance.WsAllMiniMarketsStatServe(client.eventHandler, client.errHandler)

		return err
	}, backoff.NewConstantBackOff(time.Second))
	if err != nil {
		log.Println("mini markets serve:", err)
	}

	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt, syscall.SIGTERM)

	<-exit
	close(stop)
	<-done

	fmt.Print("\nlistener was stopped")
}
