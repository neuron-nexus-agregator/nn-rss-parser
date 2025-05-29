package app

import (
	"os"
	"sync"

	endpoint "agregator/rss/internal/endpoint/app"
	"agregator/rss/internal/interfaces"
	"agregator/rss/internal/service/kafka"
	"agregator/rss/internal/service/redis"
)

type App struct {
	app    *endpoint.App
	logger interfaces.Logger
}

func New(logger interfaces.Logger) *App {
	redisCahche := redis.New(os.Getenv("REDIS_ADDR"), os.Getenv("REDIS_PASSWORD"))
	endpoint, err := endpoint.New(redisCahche, logger)
	if err != nil {
		panic(err)
	}
	return &App{
		app:    endpoint,
		logger: logger,
	}
}

func (a *App) Run() {
	output := a.app.Output()
	noFullText := a.app.NoFullText()
	kafka := kafka.New(a.logger)
	wg := sync.WaitGroup{}
	wg.Add(3)
	kafkfAddr := os.Getenv("KAFKA_ADDR")
	a.logger.Info("Starting app")
	go func() {
		defer wg.Done()
		kafka.StartWriting([]string{kafkfAddr}, "preprocessor", output)
	}()
	go func() {
		defer wg.Done()
		kafka.StartWriting([]string{kafkfAddr}, "extract-full-text", noFullText)
	}()
	go func() {
		defer wg.Done()
		a.app.Start()
	}()
	wg.Wait()
}
