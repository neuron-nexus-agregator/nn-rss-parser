package sources

import (
	"fmt"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	model "agregator/rss/internal/model/db"
)

type SourceDB struct {
	db *sqlx.DB
}

func New() (*SourceDB, error) {
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
		return nil, err
	}

	db.SetMaxOpenConns(30)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	return &SourceDB{
		db: db,
	}, nil
}

func (s *SourceDB) GetSources() ([]model.Source, error) {
	var sources []model.Source
	err := s.db.Select(&sources, "SELECT * FROM sources")
	if err != nil {
		return nil, err
	}
	return sources, nil
}

func (s *SourceDB) ChangeUpdateInterval(id int64, interval int) error {
	_, err := s.db.Exec("UPDATE sources SET update_interval = $1 WHERE id = $2", interval, id)
	return err
}
