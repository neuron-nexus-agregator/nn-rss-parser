package app

import (
	"log"
	"sync"
	"time"

	"agregator/rss/internal/model/db"
	"agregator/rss/internal/model/rss"
	"agregator/rss/internal/service/db/sources"
	"agregator/rss/internal/service/parser"
	"agregator/rss/internal/service/redis"
)

const (
	MIN_UPDATE_INTERVAL     = 10
	MAX_UPDATE_INTERVAL     = 3600
	DEFAULT_UPDATE_INTERVAL = 30
	UPDATE_STEP             = 10
)

type App struct {
	db         *sources.SourceDB
	parser     *parser.Service
	sources    []db.Source
	sourceMap  map[string]int // Мапа для быстрого доступа к индексам источников
	output     chan rss.Item
	noFullText chan rss.Item
}

// Валидирует источники и приводит их в нормальное состояние
func validateSources(sourceDB *sources.SourceDB, sourceItems ...db.Source) []db.Source {
	sourceItemsParsed := make([]db.Source, 0, len(sourceItems))
	for _, sp := range sourceItems {
		if sp.UpdateInterval < MIN_UPDATE_INTERVAL || sp.UpdateInterval > MAX_UPDATE_INTERVAL {
			sp.UpdateInterval = DEFAULT_UPDATE_INTERVAL
			go func(id int64) {
				err := sourceDB.ChangeUpdateInterval(id, DEFAULT_UPDATE_INTERVAL)
				if err != nil {
					log.Printf("Ошибка при изменении интервала обновления для источника с ID %d: %v", id, err)
				}
			}(sp.Id)
		}
		sourceItemsParsed = append(sourceItemsParsed, sp)
	}
	return sourceItemsParsed
}

// Создает новый экземпляр приложения
func New(cache *redis.RedisCache) (*App, error) {
	sourceDB, err := sources.New()
	if err != nil {
		return nil, err
	}

	// Получаем и валидируем источники из базы данных
	dbSources, err := sourceDB.GetSources()
	if err != nil {
		return nil, err
	}
	validSources := validateSources(sourceDB, dbSources...)

	if len(validSources) == 0 {
		return nil, log.Output(2, "No sources to parse") // Исправлено: убран panic
	}

	return &App{
		db:         sourceDB,
		sources:    validSources,
		output:     make(chan rss.Item, 30),
		noFullText: make(chan rss.Item, 30),
		parser:     parser.New(cache, 72*time.Hour),
	}, nil
}

// Возвращает канал с элементами, содержащими полный текст
func (a *App) Output() <-chan rss.Item {
	return a.output
}

// Возвращает канал с элементами без полного текста
func (a *App) NoFullText() <-chan rss.Item {
	return a.noFullText
}

// Обновляет мапу источников
func (a *App) updateSourceMap() {
	a.sourceMap = make(map[string]int)
	for i, source := range a.sources {
		a.sourceMap[source.Url] = i
	}
}

// Обрабатывает источник
func (a *App) startParsincBySource(source *db.Source) {
	newItems, err := a.parser.Parse(source.Url, source.Name)
	if err != nil {
		log.Printf("Ошибка при парсинге источника с ID %d: %v", source.Id, err)
		return
	}
	if len(newItems) == 0 {
		err = a.changeUpdateInterval(source, +UPDATE_STEP)
	} else {
		err = a.changeUpdateInterval(source, -UPDATE_STEP)
	}
	if err != nil {
		log.Printf("Ошибка при изменении интервала обновления для источника с ID %d: %v", source.Id, err)
	}
	for _, item := range newItems {
		if item.HasFullText {
			a.output <- item
		} else {
			a.noFullText <- item
		}
	}
}

// Изменяет интервал обновления источника
func (a *App) changeUpdateInterval(source *db.Source, change int) error {

	now := time.Now().Add(3 * time.Hour).Hour()
	if now < 7 || now > 20 {
		return nil
	}

	newInterval := source.UpdateInterval + change
	if newInterval < MIN_UPDATE_INTERVAL || newInterval > MAX_UPDATE_INTERVAL {
		return nil
	}

	source.UpdateInterval = newInterval
	return a.db.ChangeUpdateInterval(source.Id, newInterval)
}

// Запускает приложение
func (a *App) Start() {
	var mu sync.Mutex
	activeSources := make(map[string]*time.Ticker) // Хранение активных тикеров для каждого источника (по URL)

	// Запуск обработки всех текущих источников
	a.startAllSources(&mu, activeSources)

	// Обновление списка источников каждые 10 минут
	go a.updateSourcesPeriodically(&mu, activeSources)

	// Бесконечный цикл для удержания выполнения программы
	select {}
}

// Запускает обработку всех текущих источников
func (a *App) startAllSources(mu *sync.Mutex, activeSources map[string]*time.Ticker) {
	for _, source := range a.sources {
		a.startSource(source, mu, activeSources)
	}
}

// Запускает обработку одного источника
func (a *App) startSource(source db.Source, mu *sync.Mutex, activeSources map[string]*time.Ticker) {
	mu.Lock()
	if _, exists := activeSources[source.Url]; exists {
		mu.Unlock()
		return // Источник уже запущен
	}
	ticker := time.NewTicker(time.Duration(source.UpdateInterval) * time.Second)
	activeSources[source.Url] = ticker
	mu.Unlock()

	go func() {
		for range ticker.C {
			// Запускаем обработку для источника
			a.startParsincBySource(&source)

			// Обновляем интервал тикера
			mu.Lock()
			newInterval := time.Duration(source.UpdateInterval) * time.Second
			ticker.Reset(newInterval)
			mu.Unlock()
		}
	}()
}

// Останавливает обработку источника
func (a *App) stopSource(url string, mu *sync.Mutex, activeSources map[string]*time.Ticker) {
	mu.Lock()
	defer mu.Unlock()
	if ticker, exists := activeSources[url]; exists {
		ticker.Stop()
		delete(activeSources, url)
	}
}

// Обновляет список источников каждые 10 минут
func (a *App) updateSourcesPeriodically(mu *sync.Mutex, activeSources map[string]*time.Ticker) {
	for range time.NewTicker(10 * time.Minute).C {
		// Получаем обновленный список источников из базы данных
		dbSources, err := a.db.GetSources()
		if err != nil {
			log.Printf("Ошибка при получении источников из базы данных: %v", err)
			continue
		}

		// Приводим источники в нормальное состояние
		newSources := validateSources(a.db, dbSources...)

		mu.Lock()
		currentSources := make(map[string]struct{})
		for _, source := range newSources {
			currentSources[source.Url] = struct{}{}
			if _, exists := activeSources[source.Url]; !exists {
				a.startSource(source, mu, activeSources) // Запускаем обработку для новых источников
			}
		}

		// Останавливаем обработку для удаленных источников
		for url := range activeSources {
			if _, exists := currentSources[url]; !exists {
				a.stopSource(url, mu, activeSources)
			}
		}

		// Обновляем список источников и мапу
		a.sources = newSources
		a.updateSourceMap()
		mu.Unlock()
	}
}
