package app

import (
	"fmt" // Добавлен импорт fmt
	"log"
	"time"

	"agregator/rss/internal/interfaces"
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
	output     chan rss.Item
	noFullText chan rss.Item
	logger     interfaces.Logger
}

// validateSources валидирует источники и приводит их в нормальное состояние
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

// New создает новый экземпляр приложения
func New(cache *redis.RedisCache, logger interfaces.Logger) (*App, error) {
	sourceDB, err := sources.New(logger)
	if err != nil {
		return nil, err
	}

	// Получаем и валидируем источники из базы данных
	dbSources, err := sourceDB.GetSources()
	if err != nil {
		return nil, err
	}
	fmt.Println("Получено источников из базы данных:", len(dbSources))
	validSources := validateSources(sourceDB, dbSources...)
	fmt.Println("Валидированных источников:", len(validSources))

	if len(validSources) == 0 {
		// Исправлено: возвращаем явную ошибку вместо log.Output
		return nil, fmt.Errorf("no sources to parse")
	}

	return &App{
		db:         sourceDB,
		sources:    validSources,
		output:     make(chan rss.Item, 30),
		noFullText: make(chan rss.Item, 30),
		parser:     parser.New(cache, 72*time.Hour, logger),
		logger:     logger,
	}, nil
}

// Output возвращает канал с элементами, содержащими полный текст
func (a *App) Output() <-chan rss.Item {
	return a.output
}

// NoFullText возвращает канал с элементами без полного текста
func (a *App) NoFullText() <-chan rss.Item {
	return a.noFullText
}

// startParsincBySource обрабатывает источник
func (a *App) startParsincBySource(source *db.Source) {
	newItems, err := a.parser.Parse(source.Url, source.Name)
	if err != nil {
		a.logger.Error("Ошибка при парсинге источника", "id", source.Id, "error", err)
		err = a.changeUpdateInterval(source, +UPDATE_STEP)
		if err != nil {
			a.logger.Error("Ошибка при изменении интервала обновления", "id", source.Id, "error", err)
		}
		return
	}
	if len(newItems) == 0 {
		err = a.changeUpdateInterval(source, +2*UPDATE_STEP)
	} else {
		err = a.changeUpdateInterval(source, -UPDATE_STEP)
	}
	if err != nil {
		a.logger.Error("Ошибка при изменении интервала обновления", "id", source.Id, "error", err)
	}
	for _, item := range newItems {
		if item.HasFullText {
			a.output <- item
		} else {
			a.noFullText <- item
		}
	}
}

// changeUpdateInterval изменяет интервал обновления источника
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

// Start запускает приложение
func (a *App) Start() {
	// Исправлено: итерируемся по индексу, чтобы получить адрес элемента
	for i := range a.sources {
		a.startSource(&a.sources[i]) // Передаем указатель на элемент
	}

	// Бесконечный цикл для удержания выполнения программы
	select {}
}

// startSource запускает обработку одного источника
// Исправлено: теперь принимает указатель на db.Source
func (a *App) startSource(source *db.Source) {
	ticker := time.NewTicker(time.Duration(source.UpdateInterval) * time.Second)

	// Исправлено: передаем source как s *db.Source в горутину
	go func(ticker *time.Ticker, s *db.Source) {
		a.startParsincBySource(s)
		for range ticker.C {
			// Запускаем обработку для источника
			go func(id int64) {
				err := a.db.SetLastRead(id)
				if err != nil {
					a.logger.Error("Ошибка при установке времени последнего обновления", "id", id, "error", err)
				}
			}(s.Id) // Используем s.Id
			a.startParsincBySource(s) // Передаем указатель s

			// Обновляем интервал тикера
			newInterval := time.Duration(s.UpdateInterval) * time.Second // Используем s.UpdateInterval
			ticker.Reset(newInterval)
		}
	}(ticker, source) // Передаем source (который уже является указателем)
}
