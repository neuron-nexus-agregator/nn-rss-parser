package parser

import (
	"sync"
	"time"

	"github.com/mmcdole/gofeed"

	"agregator/rss/internal/interfaces"
	model "agregator/rss/internal/model/rss"
	"agregator/rss/internal/service/redis"
)

type Service struct {
	parser *gofeed.Parser
	cahce  *redis.RedisCache
	dur    time.Duration
	logger interfaces.Logger
}

func New(cahce *redis.RedisCache, maxTimeDiffrance time.Duration, logger interfaces.Logger) *Service {
	return &Service{
		parser: gofeed.NewParser(),
		cahce:  cahce,
		dur:    maxTimeDiffrance,
		logger: logger,
	}
}
func (s *Service) Parse(url, sourceName string) (newItems []model.Item, err error) {
	feed, err := s.parser.ParseURL(url)
	if err != nil {
		s.logger.Error("Ошибка при парсинге URL", "url", url, "error", err)
		s.logProblematicPage(url)
		return nil, err
	}

	itemsChan := make(chan model.Item, len(feed.Items))
	var wg sync.WaitGroup

	for _, item := range feed.Items {
		wg.Add(1)
		if item.PublishedParsed == nil {
			continue
		}
		date := *item.PublishedParsed
		if date.Day() != time.Now().Day() {
			continue
		}
		go func(item *gofeed.Item) {
			defer wg.Done()
			processedItem, ok := s.processItem(sourceName, item)
			if ok {
				itemsChan <- processedItem
			}
		}(item)
	}

	go func() {
		wg.Wait()
		close(itemsChan)
	}()

	for item := range itemsChan {
		if !s.validateURL(item) {
			continue
		}
		if ok, text, err := s.cahce.Exists("rss:" + item.MD5); err == nil && ok {
			if text != item.FullText {
				item.Changed = true
			}
		} else if err != nil {
			s.logger.Error("Ошибка при проверке кеша", "error", err)
			continue
		} else {
			err = s.cahce.Set("rss:"+item.MD5, item.FullText)
			if err != nil {
				s.logger.Error("Ошибка при записи в кеш", "error", err)
				continue
			}

		}
		newItems = append(newItems, item)
	}
	return newItems, nil
}
