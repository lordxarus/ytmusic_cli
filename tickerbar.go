package main

import (
	"fmt"
	"log"
	"math"
	"time"

	"code.rocketnine.space/tslocum/cview"
)

type tickerBar struct {
	app *cview.Application
	bar *cview.ProgressBar

	ticker    *time.Ticker
	killChan  chan bool
	isRunning bool

	startTime time.Time
}

func newTickerBar(
	app *cview.Application,
	bar *cview.ProgressBar,
) *tickerBar {
	return &tickerBar{
		app:      app,
		bar:      bar,
		ticker:   time.NewTicker(time.Millisecond * 500),
		killChan: make(chan bool, 1),
	}
}

func (spb *tickerBar) start(duration time.Duration) {
	log.Println("tickerBar: start()")
	startTime := time.Now()
	spb.isRunning = true
	go func() {
		for {
			select {
			case <-spb.killChan:
				spb.isRunning = false
				log.Println("tickerBar: runner: killed after",
					fmt.Sprintf("%.2f", time.Since(spb.startTime).Seconds()),
					"seconds")
				return
			case <-spb.ticker.C:
				elapsed := float64(time.Since(startTime)) / float64(duration)
				percentElapsed := int(math.Floor(elapsed * 100))
				// log.Println("tickerBar: runner: elapsed: ", percentElapsed)
				spb.bar.SetProgress(percentElapsed)
				app.Draw(spb.bar)
				// log.Println("tickerBar: runner: ", int(elapsed/songLength))
			}
		}
	}()
}

func (spb *tickerBar) stop() {
	log.Println("songProgressBar: stop()")
	if spb.isRunning {
		spb.killChan <- true
	}
}

func (spb *tickerBar) IsRunning() bool {
	return spb.isRunning
}
