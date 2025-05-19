package parser

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	model "agregator/rss/internal/model/rss"
)

// check проверяет валидность элемента и возвращает полный текст и статус.
func (s *Service) check(item *gofeed.Item) (string, bool) {
	fullText, got := extractFullText(item)

	// Если полный текст отсутствует, используем описание с предупреждением.
	if !got {
		fullText = "<p>" + item.Description + "</p><p><b>Доступно только описание. Полный текст — в первоисточнике</b></p>"
	}

	return fullText, got
}

// md5 генерирует MD5-хэш для ссылки элемента.
func (s *Service) md5(item *gofeed.Item) string {
	var textToMD5 string
	if item.GUID != "" {
		textToMD5 = item.GUID
	} else {
		textToMD5 = item.Link
	}
	textToHash := strings.TrimSpace(strings.ToLower(textToMD5))
	hash := md5.Sum([]byte(textToHash))
	return hex.EncodeToString(hash[:])
}

// extractFullText извлекает полный текст из элемента.
func extractFullText(item *gofeed.Item) (string, bool) {
	// Проверяем наличие расширения "full-text".
	for _, val := range item.Extensions {
		if ext, exists := val["full-text"]; exists && len(ext) > 0 {
			return ext[0].Value, true
		}
	}

	// Если расширение отсутствует, используем контент.
	if item.Content != "" {
		return item.Content, true
	}

	// Если контент отсутствует, возвращаем пустую строку.
	return "", false
}

func (s *Service) processItem(sourceName string, item *gofeed.Item) (model.Item, bool) {
	now := time.Now()
	if item.PublishedParsed == nil {
		return model.Item{}, false
	}
	if now.Sub(*item.PublishedParsed) > s.dur {
		return model.Item{}, false
	}
	fullText, got := s.check(item)

	category := ""
	if len(item.Categories) > 0 {
		category = item.Categories[0]
	}

	return model.Item{
		Title:       strings.TrimSpace(item.Title),
		Description: strings.TrimSpace(item.Description),
		PubDate:     item.PublishedParsed,
		FullText:    strings.TrimSpace(fullText),
		MD5:         s.md5(item),
		Link:        item.Link,
		Name:        strings.TrimSpace(sourceName),
		Enclosure:   s.getEnclosure(item),
		Category:    category,
		HasFullText: got,
	}, true
}

func (s *Service) getEnclosure(item *gofeed.Item) string {
	if item.Enclosures != nil {
		if len(item.Enclosures) > 0 {
			for _, enc := range item.Enclosures {
				if strings.Contains(enc.Type, "image/") {
					return enc.URL
				}
			}
		}
	}
	return ""
}

func (s *Service) logProblematicPage(url string) {
	res, err := http.Get(url)
	if err != nil {
		log.Printf("Ошибка HTTP GET для URL: %s, ошибка: %v\n", url, err)
		return
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("Ошибка при чтении тела ответа: %v\n", err)
		return
	}

	log.Printf("Содержимое проблемной страницы (первые 500 символов): %s\n", string(data[:500]))
}

func (s *Service) validateURL(item model.Item) bool {
	// Список запрещенных подстрок
	disallowedSubstrings := []string{
		"erid=", "/video/", "/photo/", "/audio/", "/gallery/", "/photoslider/", "/photos/", "/videos/", "/audios/", "/galleries/", "/podcast/", "/podcasts/",
	}

	// Проверяем, содержит ли ссылка запрещенные подстроки
	for _, substr := range disallowedSubstrings {
		if strings.Contains(item.Link, substr) {
			return false
		}
	}

	req, err := http.NewRequest(http.MethodGet, item.Link, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		return false
	}
	defer res.Body.Close()

	return true
}
