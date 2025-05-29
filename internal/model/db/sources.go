package db

import "time"

type Source struct {
	Id             int64      `db:"id"`
	Name           string     `db:"name"`
	Url            string     `db:"url"`
	UpdateInterval int        `db:"update_interval"`
	Relevance      float64    `db:"relevance"`
	LastRead       *time.Time `db:"last_read"`
}
