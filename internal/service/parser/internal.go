package parser

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"strings"

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

	// Если расширение отсутствует, используем контент.
	if len(item.Content) > 0 {
		return item.Content, true
	}

	// Проверяем наличие расширения "full-text".
	for _, val := range item.Extensions {
		if ext, exists := val["full-text"]; exists && len(ext) > 0 {
			return ext[0].Value, true
		} else if ext, exists = val["full_text"]; exists && len(ext) > 0 {
			return ext[0].Value, true
		} else if ext, exists = val["fulltext"]; exists && len(ext) > 0 {
			return ext[0].Value, true
		} else if ext, exists = val["fullText"]; exists && len(ext) > 0 {
			return ext[0].Value, true
		}
	}

	// Если контент отсутствует, возвращаем пустую строку.
	return "", false
}

func (s *Service) processItem(sourceName string, item *gofeed.Item) (model.Item, bool) {
	//now := time.Now()
	if item.PublishedParsed == nil {
		return model.Item{}, false
	}
	// if now.Sub(*item.PublishedParsed) > s.dur {
	// 	return model.Item{}, false
	// }
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
		s.logger.Error("Ошибка HTTP GET для URL", "url", url, "error", err)
		return
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		s.logger.Error("Ошибка при чтении тела ответа", "url", url, "error", err)
		return
	}
	if len(data) > 500 {
		s.logger.Info("Проблемная страница", "url", url, "data", string(data[:500]))
	} else {
		s.logger.Info("Проблемная страница", "url", url, "data", string(data))
	}
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

	return true
}
