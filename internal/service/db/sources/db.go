package sources

import (
	"fmt"
	"os"
	"time"

	"agregator/rss/internal/interfaces"
	model "agregator/rss/internal/model/db"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type SourceDB struct {
	db     *sqlx.DB
	logger interfaces.Logger
}

func New(logger interfaces.Logger) (*SourceDB, error) {
	connectionData := fmt.Sprintf(
		"user=%s dbname=%s sslmode=disable password=%s host=%s port=%s",
		os.Getenv("DB_LOGIN"),
		"newagregator", // os.Getenv("DB_NAME"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
	)

	db, err := sqlx.Connect("postgres", connectionData)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		return nil, err
	}

	db.SetMaxOpenConns(30)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	return &SourceDB{
		db:     db,
		logger: logger,
	}, nil
}

func (s *SourceDB) GetSources() ([]model.Source, error) {
	var sources []model.Source
	err := s.db.Select(&sources, "SELECT * FROM sources")
	if err != nil {
		s.logger.Error("Failed to get sources", "error", err)
		return nil, err
	}
	return sources, nil
}

func (s *SourceDB) ChangeUpdateInterval(id int64, interval int) error {
	_, err := s.db.Exec("UPDATE sources SET update_interval = $1 WHERE id = $2", interval, id)
	return err
}

func (s *SourceDB) SetLastRead(id int64) error {
	now := time.Now()
	_, err := s.db.Exec("UPDATE sources SET last_read = $1 WHERE id = $2", now, id)
	if err != nil {
		s.logger.Error("Failed to set last read", "error", err)
	}
	return err
}
