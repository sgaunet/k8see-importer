// Package database provides database migration and connection utilities.
package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/sgaunet/dsn/v2/pkg/dsn"
)

const dbPingRetryDelay = 200 * time.Millisecond

//go:embed migrations/*.sql
var embedMigrations embed.FS

// Migrate runs database migrations using the embedded SQL files.
func Migrate(db *sql.DB) error {
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	_, err := goose.EnsureDBVersion(db)
	if err != nil {
		return err
	}
	return goose.Up(db, "migrations")
}

// WaitForDB waits for the database to be ready.
func WaitForDB(ctx context.Context, pgdsn string) error {
	d, err := dsn.New(pgdsn)
	if err != nil {
		return err
	}
	chDBReady := make(chan struct{})
	go func() {
		for {
			db, err := sql.Open("postgres", d.GetPostgresUri())
			select {
			case <-ctx.Done():
				return
			default:
				if err == nil {
					err = db.PingContext(ctx)
					_ = db.Close()
					if err == nil {
						fmt.Println("Database ready")
						close(chDBReady)
						return
					}
				}
				// fmt.Println("Waiting for database to be ready...", pgdsn, err.Error())
				time.Sleep(dbPingRetryDelay)
			}
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-chDBReady:
		return nil
	}
}
