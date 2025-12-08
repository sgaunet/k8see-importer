// Package main is the entry point for the k8see-importer application.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/rs/xid"
	"github.com/sgaunet/k8see-importer/internal/config"
	"github.com/sgaunet/k8see-importer/internal/database"
	"github.com/sirupsen/logrus"
)

const (
	consumersGroup         string = "k8see-consumer-group"
	dbPingRetryDelay              = 200 * time.Millisecond
	redisConnectRetryDelay        = 2 * time.Second
	dbConnectionTimeout           = 30 * time.Second
	redisReadCount         int64  = 2
	purgeInterval                 = 24 * time.Hour
	shutdownTimeout               = 10 * time.Second
	shutdownGracePeriod           = 100 * time.Millisecond
	maxOpenConns                  = 25
	maxIdleConns                  = 5
	connMaxLifetime               = 5 * time.Minute
)

type appK8sRedis2Db struct {
	redisHost           string
	redisPort           string
	redisPassword       string
	redisStream         string
	dbHost              string
	dbPort              string
	dbUser              string
	dbPassword          string
	dbName              string
	dataRetentionInDays int
	redisClient         *redis.Client
	redisClientMu       sync.Mutex
	dbConn              *sql.DB
	shutdownChan        chan struct{}
}

var log = logrus.New()

// initTrace initialize log instance with the level in parameter.
func initTrace(debugLevel string) {
	log.SetOutput(os.Stdout)

	switch debugLevel {
	case "info":
		log.SetLevel(logrus.InfoLevel)
	case "warn":
		log.SetLevel(logrus.WarnLevel)
	case "error":
		log.SetLevel(logrus.ErrorLevel)
	default:
		log.SetLevel(logrus.DebugLevel)
	}
}

func loadConfig() *config.YamlConfig {
	var fileConfigName string
	flag.StringVar(&fileConfigName, "f", "", "YAML file to parse.")
	flag.Parse()

	var cfg *config.YamlConfig
	var err error

	if fileConfigName == "" {
		log.Infoln("No config file specified.")
		log.Infoln("Try to get configuration with environment variable")
		cfg = config.LoadConfigFromEnv()
	} else {
		cfg, err = config.LoadConfigFromFile(fileConfigName)
		if err != nil {
			log.Fatal(err)
		}
	}

	if err := cfg.IsConfigValid(); err != nil {
		log.Errorln(err.Error())
		os.Exit(1)
	}

	return cfg
}

func runProcessingLoop(ctx context.Context, app *appK8sRedis2Db, done chan struct{}) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			log.Infoln("Shutting down gracefully...")
			return
		default:
			time.Sleep(redisConnectRetryDelay)
			err := app.InitConsumer(ctx)
			if err != nil {
				log.Errorln(err.Error())
				continue
			}

			// Call the function that have an infinite loop
			err = app.redis2PG(ctx)
			if err != nil {
				log.Errorln(err.Error())
				continue
			}
		}
	}
}

func waitForShutdown(cancel context.CancelFunc, done chan struct{}) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Infoln("Received shutdown signal")
	cancel()

	select {
	case <-done:
		log.Infoln("Shutdown complete")
	case <-time.After(shutdownTimeout):
		log.Warnln("Shutdown timeout exceeded, forcing exit")
	}
}

func main() {
	cfg := loadConfig()
	initTrace(cfg.LogLevel)

	db, err := initDB(cfg)
	if err != nil {
		log.Fatalln(err.Error())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := NewApp(*cfg, db)
	done := make(chan struct{})

	go runProcessingLoop(ctx, app, done)
	waitForShutdown(cancel, done)

	// Signal background goroutines to stop
	close(app.shutdownChan)

	// Give background tasks a moment to clean up
	time.Sleep(shutdownGracePeriod)

	// Close database connection on shutdown
	if err := app.dbConn.Close(); err != nil {
		log.Errorf("Error closing database: %v", err)
	}

	// Close Redis connection if exists
	app.redisClientMu.Lock()
	if app.redisClient != nil {
		if err := app.redisClient.Close(); err != nil {
			log.Errorf("Error closing Redis: %v", err)
		}
	}
	app.redisClientMu.Unlock()
}

func initDB(cfg *config.YamlConfig) (*sql.DB, error) {
	pgdsn := fmt.Sprintf(
		"postgres://%s:%s@%s/%s?sslmode=disable",
		cfg.DbUser,
		cfg.DbPassword,
		net.JoinHostPort(cfg.DbHost, cfg.DbPort),
		cfg.DbName,
	)
	log.Infoln("Wait for database connection")
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(dbConnectionTimeout))
	defer cancel()
	err := database.WaitForDB(ctx, pgdsn)
	if err != nil {
		return nil, err
	}

	log.Infoln("Database is ready")
	log.Infoln("Creating database schema and running migrations")
	db, err := sql.Open("postgres", pgdsn)
	if err != nil {
		return nil, err
	}

	// Configure connection pool
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	// Verify connection
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := database.Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

// NewApp is the factory to get a new instance of the application.
func NewApp(cfg config.YamlConfig, db *sql.DB) *appK8sRedis2Db {
	app := appK8sRedis2Db{
		redisHost:           cfg.RedisHost,
		redisPort:           cfg.RedisPort,
		redisPassword:       cfg.RedisPassword,
		redisStream:         cfg.RedisStream,
		dbHost:              cfg.DbHost,
		dbPort:              cfg.DbPort,
		dbName:              cfg.DbName,
		dbUser:              cfg.DbUser,
		dbPassword:          cfg.DbPassword,
		dataRetentionInDays: cfg.DataRetentionInDays,
		dbConn:              db,
		shutdownChan:        make(chan struct{}),
	}
	go app.BackgroundPurge()
	return &app
}

// BackgroundPurge runs PurgeDB periodically in the background.
func (a *appK8sRedis2Db) BackgroundPurge() {
	_ = a.PurgeDB()
	tick := time.NewTicker(purgeInterval)
	defer tick.Stop()
	for {
		select {
		case <-a.shutdownChan:
			log.Infoln("BackgroundPurge: Shutting down")
			return
		case <-tick.C:
			_ = a.PurgeDB()
		}
	}
}

// InitConsumer initialise redisClient.
func (a *appK8sRedis2Db) InitConsumer(ctx context.Context) error {
	a.redisClientMu.Lock()
	defer a.redisClientMu.Unlock()

	// Close existing client if it exists
	if a.redisClient != nil {
		_ = a.redisClient.Close()
		a.redisClient = nil
	}

	addr := fmt.Sprintf("%s:%s", a.redisHost, a.redisPort)
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: a.redisPassword,
	})

	_, err := client.Ping(ctx).Result()
	if err != nil {
		_ = client.Close()
		return fmt.Errorf("redis ping failed: %w", err)
	}

	// Only set if successful
	a.redisClient = client

	log.Infoln("Connected to Redis server")
	err = a.redisClient.XGroupCreate(ctx, a.redisStream, consumersGroup, "0").Err()
	if err != nil {
		log.Errorln(err.Error())
	}
	return nil
}

// PurgeDB removes old k8s events from the database.
func (a *appK8sRedis2Db) PurgeDB() error {
	ctx := context.Background()
	sqlStatement := "DELETE FROM k8sevents WHERE exportedTime <= now() - $1 * INTERVAL '1 DAY'"
	r, err := a.dbConn.ExecContext(ctx, sqlStatement, a.dataRetentionInDays)
	if err != nil {
		log.Errorf("Delete failed : %s\n", err.Error())
		return err
	}
	rowsAffected, err := r.RowsAffected()
	if err == nil {
		log.Infof("PurgeDB : Deleted %d rows", rowsAffected)
	}
	return err
}

var (
	errInvalidEventTime      = errors.New("invalid eventTime")
	errInvalidFirstTime      = errors.New("invalid firstTime")
	errInvalidExportedTime   = errors.New("invalid exportedTime")
	errInvalidName           = errors.New("invalid name")
	errRedisClientNotInitialized = errors.New("redis client not initialized")
)

type eventData struct {
	eventTime    time.Time
	firstTime    time.Time
	exportedTime time.Time
	name         string
	reason       string
	eventType    string
	message      string
	namespace    string
}

func extractEventData(values map[string]any) (*eventData, error) {
	eventTimeStr, ok := values["eventTime"].(string)
	if !ok {
		return nil, errInvalidEventTime
	}
	firstTimeStr, ok := values["firstTime"].(string)
	if !ok {
		return nil, errInvalidFirstTime
	}
	exportedTimeStr, ok := values["exportedTime"].(string)
	if !ok {
		return nil, errInvalidExportedTime
	}
	nameStr, ok := values["name"].(string)
	if !ok {
		return nil, errInvalidName
	}

	reasonStr, _ := values["reason"].(string)
	typeStr, _ := values["type"].(string)
	messageStr, _ := values["message"].(string)
	namespaceStr, _ := values["namespace"].(string)

	return &eventData{
		eventTime:    convertStrToTime(eventTimeStr),
		firstTime:    convertStrToTime(firstTimeStr),
		exportedTime: convertStrToTime(exportedTimeStr),
		name:         nameStr,
		reason:       reasonStr,
		eventType:    typeStr,
		message:      messageStr,
		namespace:    namespaceStr,
	}, nil
}

func (a *appK8sRedis2Db) processMessage(ctx context.Context, messageID string, values map[string]any) {
	event, err := extractEventData(values)
	if err != nil {
		log.Warnf("Skipping message %s: %v", messageID, err)
		return
	}

	log.Infof("NEW=> type=%s reason=%s name=%s", event.eventType, event.reason, event.name)
	log.Debugf("eventTime=%v firstTime=%v exportedTime=%v", event.eventTime, event.firstTime, event.exportedTime)

	err = a.addUpdateSubcode(
		ctx, event.exportedTime, event.eventTime, event.firstTime,
		event.name, event.reason, event.eventType, event.message, event.namespace,
	)
	if err != nil {
		log.Errorln(err.Error())
		return
	}

	if a.redisClient == nil {
		log.Warnln("Cannot ACK message: Redis client is nil")
		return
	}

	if err := a.redisClient.XAck(ctx, a.redisStream, consumersGroup, messageID).Err(); err != nil {
		log.Warnf("Failed to acknowledge message %s: %v", messageID, err)
	}
}

func (a *appK8sRedis2Db) redis2PG(ctx context.Context) error {
	if a.redisClient == nil {
		return errRedisClientNotInitialized
	}

	uniqueID := xid.New().String()
	for {
		select {
		case <-ctx.Done():
			log.Infoln("redis2PG: Context cancelled, stopping...")
			return ctx.Err()
		default:
		}

		entries, err := a.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumersGroup,
			Consumer: uniqueID,
			Streams:  []string{a.redisStream, ">"},
			Count:    redisReadCount,
			Block:    0,
			NoAck:    false,
		}).Result()
		if err != nil {
			return err
		}

		for i := range entries[0].Messages {
			a.processMessage(ctx, entries[0].Messages[i].ID, entries[0].Messages[i].Values)
		}
	}
}

func convertStrToTime(str string) time.Time {
	layout := "2006-01-02 15:04:05 -0700 MST"
	// 0001-01-01 00:00:00 +0000 UTC
	newTime, err := time.Parse(layout, str)

	if err != nil {
		log.Errorln("convertStrToTime:", err.Error())
	}
	return newTime
}

func (a *appK8sRedis2Db) addUpdateSubcode(
	ctx context.Context,
	exportedTime time.Time,
	firstTime time.Time,
	eventTime time.Time,
	eventName string,
	eventReason string,
	eventType string,
	eventMessage string,
	eventNamespace string,
) error {
	sqlStatement := `
INSERT INTO k8sevents (exportedTime,firstTime,eventTime,name, reason, type,message,namespace)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := a.dbConn.ExecContext(
		ctx,
		sqlStatement,
		exportedTime, firstTime, eventTime,
		eventName, eventReason, eventType, eventMessage, eventNamespace,
	)
	if err != nil {
		log.Errorf("Insert failed : %s\n", err.Error())
		log.Debugln("exportedTime=", exportedTime)
		log.Debugln("firstTime=", firstTime)
		log.Debugln("eventTime=", eventTime)
		log.Debugln("eventName=", eventName)
		log.Debugln("eventReason=", eventReason)
		log.Debugln("eventType=", eventType)
		log.Debugln("eventMessage=", eventMessage)
		log.Debugln("eventNamespace=", eventNamespace)
		return err
	}
	return nil
}
