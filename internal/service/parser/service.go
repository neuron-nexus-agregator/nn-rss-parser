package parser

import (
	"log"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"

	model "agregator/rss/internal/model/rss"
	"agregator/rss/internal/service/redis"
)

type Service struct {
	parser *gofeed.Parser
	cahce  *redis.RedisCache
	dur    time.Duration
}

func New(cahce *redis.RedisCache, maxTimeDiffrance time.Duration) *Service {
	return &Service{
		parser: gofeed.NewParser(),
		cahce:  cahce,
		dur:    maxTimeDiffrance,
	}
}
func (s *Service) Parse(url, sourceName string) (newItems []model.Item, err error) {
	feed, err := s.parser.ParseURL(url)
	if err != nil {
		log.Printf("Ошибка при парсинге URL: %s, ошибка: %v\n", url, err)
		s.logProblematicPage(url)
		return nil, err
	}

	itemsChan := make(chan model.Item, len(feed.Items))
	var wg sync.WaitGroup

	for _, item := range feed.Items {
		wg.Add(1)
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
		if ok, text, err := s.cahce.Exists(item.MD5); err == nil && ok {
			if text != item.FullText {
				item.Changed = true
			}
		} else if err != nil {
			log.Printf("Ошибка при проверке кеша: %v\n", err)
			continue
		} else {
			err = s.cahce.Set(item.MD5, item.FullText)
			if err != nil {
				log.Printf("Ошибка при записи в кеш: %v\n", err)
				continue
			}

		}
		newItems = append(newItems, item)
	}
	return newItems, nil
}
