// Package main is the entry point for the k8see-importer application.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
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
}

var log = logrus.New()
var cnx *sql.DB

// initTrace initialize log instance with the level in parameter.
func initTrace(debugLevel string) {
	// Log as JSON instead of the default ASCII formatter.
	// log.SetFormatter(&log.JSONFormatter{})
	// log.SetFormatter(&log.TextFormatter{
	// 	DisableColors: true,
	// 	FullTimestamp: true,
	// })

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
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

	if err := initDB(); err != nil {
		log.Fatalln(err.Error())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := NewApp(*cfg)
	done := make(chan struct{})

	go runProcessingLoop(ctx, app, done)
	waitForShutdown(cancel, done)
}

func initDB() error {
	dbUser := os.Getenv("DBUSER")
	dbPassword := os.Getenv("DBPASSWORD")
	dbHost := os.Getenv("DBHOST")
	dbPort := os.Getenv("DBPORT")
	dbName := os.Getenv("DBNAME")
	pgdsn := fmt.Sprintf(
		"postgres://%s:%s@%s/%s?sslmode=disable",
		dbUser,
		dbPassword,
		net.JoinHostPort(dbHost, dbPort),
		dbName,
	)
	log.Infoln("INFO: Wait for database connection")
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(dbConnectionTimeout))
	defer cancel()
	err := database.WaitForDB(ctx, pgdsn)
	if err != nil {
		return err
	}

	log.Infoln("INFO: Database is ready")
	log.Infoln("INFO: create the database (if it does not already exist) and run any pending migrations")
	db, err := sql.Open("postgres", pgdsn)
	if err != nil {
		return err
	}
	return database.Migrate(db)
}

// NewApp is the factory to get a new instance of the application.
func NewApp(cfg config.YamlConfig) *appK8sRedis2Db {
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
	}
	go app.BackgroundPurge()
	return &app
}

// BackgroundPurge runs PurgeDB periodically in the background.
func (a *appK8sRedis2Db) BackgroundPurge() {
	_ = a.PurgeDB()
	tick := time.NewTicker(purgeInterval)
	for range tick.C {
		_ = a.PurgeDB()
	}
}

// InitConsumer initialise redisClient.
func (a *appK8sRedis2Db) InitConsumer(ctx context.Context) error {
	var err error
	addr := fmt.Sprintf("%s:%s", a.redisHost, a.redisPort)
	a.redisClient = redis.NewClient(&redis.Options{
		Addr: addr,
	})
	_, err = a.redisClient.Ping(ctx).Result()
	if err != nil {
		return err
	}
	log.Infoln("Connected to Redis server")
	err = a.redisClient.XGroupCreate(ctx, a.redisStream, consumersGroup, "0").Err()
	if err != nil {
		log.Errorln(err.Error())
	}
	return nil
}

// PurgeDB removes old k8s events from the database.
func (a *appK8sRedis2Db) PurgeDB() error {
	var err error
	ctx := context.Background()
	if !a.isConnectedToDB(ctx) {
		cnx, err = a.cnxDB(ctx)
		if err != nil {
			log.Errorln(err.Error())
			return err
		}
	}
	sqlStatement := "DELETE FROM k8sevents WHERE exportedTime <= now() - $1 * INTERVAL '1 DAY'"
	r, err := cnx.ExecContext(ctx, sqlStatement, a.dataRetentionInDays)
	if err != nil {
		log.Errorf("Delete failed : %s\n", err.Error())
		return err
	}
	rowsAffected, err := r.RowsAffected()
	if err == nil {
		log.Infof("PurgeDB : Delete %d rows", rowsAffected)
	}
	return err
}

func (a *appK8sRedis2Db) redis2PG(ctx context.Context) error {
	var err error
	cnx, err = a.cnxDB(ctx)
	if err != nil {
		return err
	}
	err = a.InitConsumer(ctx)
	if err != nil {
		return err
	}

	uniqueID := xid.New().String()
	for {
		// Check for shutdown signal
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
			messageID := entries[0].Messages[i].ID
			values := entries[0].Messages[i].Values
			eventTimeStr, _ := values["eventTime"].(string)
			firstTimeStr, _ := values["firstTime"].(string)
			exportedTimeStr, _ := values["exportedTime"].(string)
			eventTime := convertStrToTime(eventTimeStr)
			firstTime := convertStrToTime(firstTimeStr)
			exportedTime := convertStrToTime(exportedTimeStr)
			nameStr, _ := values["name"].(string)
			reasonStr, _ := values["reason"].(string)
			typeStr, _ := values["type"].(string)
			messageStr, _ := values["message"].(string)
			namespaceStr, _ := values["namespace"].(string)
			log.Infof("NEW=> type=%s reason=%s name=%s\n", typeStr, reasonStr, nameStr)
			log.Infof("      eventTime=%s firstTime=%s \n", eventTimeStr, firstTimeStr)
			log.Debugln("eventTime=", eventTime)
			log.Debugln("firstTime=", firstTime)
			log.Debugln("exportedTime=", exportedTime)
			err = a.addUpdateSubcode(
				ctx, exportedTime, eventTime, firstTime,
				nameStr, reasonStr, typeStr, messageStr, namespaceStr,
			)
			if err != nil {
				log.Errorln(err.Error())
			} else {
				a.redisClient.XAck(ctx, a.redisStream, consumersGroup, messageID)
			}
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

func (a *appK8sRedis2Db) cnxDB(ctx context.Context) (*sql.DB, error) {
	nbtries := 3
	var db *sql.DB
	var err error

	for try := 1; try <= nbtries && !a.isConnectedToDB(ctx); try++ {
		psqlInfo := fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			a.dbHost, a.dbPort, a.dbUser, a.dbPassword, a.dbName,
		)
		db, err = sql.Open("postgres", psqlInfo)
		if err != nil {
			log.Errorf("Failed to connect to database (attempt %d/%d): %v", try, nbtries, err)
			if try == nbtries {
				return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", nbtries, err)
			}
			time.Sleep(dbPingRetryDelay)
		}
	}

	return db, err
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
	var err error
	if !a.isConnectedToDB(ctx) {
		cnx, err = a.cnxDB(ctx)
		if err != nil {
			log.Errorln(err.Error())
			return err
		}
	}
	sqlStatement := `
INSERT INTO k8sevents (exportedTime,firstTime,eventTime,name, reason, type,message,namespace)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err = cnx.ExecContext(
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
		log.Debugln("eventNamespace=", eventName)
		return err
	}
	return err
}

func (a *appK8sRedis2Db) isConnectedToDB(ctx context.Context) bool {
	if cnx == nil {
		return false
	}
	err := cnx.PingContext(ctx)
	if err != nil {
		log.Errorln("Not connected to DB")
		return false
	}
	return true
}
