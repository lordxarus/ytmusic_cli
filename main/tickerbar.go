package main

import (
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
	startTime := time.Now()
	spb.isRunning = true
	go func() {
		for {
			select {
			case <-spb.killChan:
				spb.isRunning = false
				return
			case <-spb.ticker.C:
				elapsed := float64(time.Since(startTime)) / float64(duration)
				percentElapsed := int(math.Floor(elapsed * 100))
				spb.bar.SetProgress(percentElapsed)
				app.Draw(spb.bar)
			}
		}
	}()
}

func (spb *tickerBar) stop() {
	if spb.isRunning {
		spb.killChan <- true
	}
}

func (spb *tickerBar) IsRunning() bool {
	return spb.isRunning
}
