package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sgaunet/k8see-importer/internal/config"
	"github.com/sgaunet/k8see-importer/internal/database"

	"database/sql"

	_ "github.com/lib/pq"

	"github.com/rs/xid"
	"github.com/sirupsen/logrus"
)

const consumersGroup string = "k8see-consumer-group"

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

// initTrace initialize log instance with the level in parameter
func initTrace(debugLevel string) {
	// Log as JSON instead of the default ASCII formatter.
	//log.SetFormatter(&log.JSONFormatter{})
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

func main() {
	var err error
	var cfg *config.YamlConfig
	var fileConfigName string
	flag.StringVar(&fileConfigName, "f", "", "YAML file to parse.")
	flag.Parse()

	if fileConfigName == "" {
		log.Infoln("No config file specified.")
		log.Infoln("Try to get configuration with environment variable")
		cfg = config.LoadConfigFromEnv()
	} else {
		cfg, err = config.LoadConfigFromFile(fileConfigName)
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}
	}

	if err := cfg.IsConfigValid(); err != nil {
		log.Errorln(err.Error())
		os.Exit(1)
	}

	initTrace(cfg.LogLevel)

	err = initDB()
	if err != nil {
		log.Fatalln(err.Error())
		os.Exit(1)
	}

	app := NewApp(*cfg)
	for {
		time.Sleep(2 * time.Second)
		err = app.InitConsumer()
		if err != nil {
			log.Errorln(err.Error())
			continue
		}

		// Call the function that have an infinite loop
		err = app.redis2PG()
		if err != nil {
			log.Errorln(err.Error())
			continue
		}
	}
}

func initDB() error {
	pgdsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", os.Getenv("DBUSER"), os.Getenv("DBPASSWORD"), os.Getenv("DBHOST"), os.Getenv("DBPORT"), os.Getenv("DBNAME"))
	log.Infoln("INFO: Wait for database connection")
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(30*time.Second))
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

// NewApp is the factory to get a new instance of the application
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

// InitConsumer initialise redisClient
func (a *appK8sRedis2Db) InitConsumer() error {
	var err error
	ctx := context.TODO()
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

func (a *appK8sRedis2Db) redis2PG() error {
	var err error
	ctx := context.TODO()
	cnx, err = a.cnxDB()
	if err != nil {
		return err
	}
	err = a.InitConsumer()
	if err != nil {
		return err
	}

	uniqueID := xid.New().String()
	for {
		entries, err := a.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumersGroup,
			Consumer: uniqueID,
			Streams:  []string{a.redisStream, ">"},
			Count:    2,
			Block:    0,
			NoAck:    false,
		}).Result()
		if err != nil {
			return err
		}
		for i := 0; i < len(entries[0].Messages); i++ {
			messageID := entries[0].Messages[i].ID
			values := entries[0].Messages[i].Values
			eventTime := convertStrToTime(values["eventTime"].(string))
			firstTime := convertStrToTime(values["firstTime"].(string))
			exportedTime := convertStrToTime(values["exportedTime"].(string))
			log.Infof("NEW=> type=%s reason=%s name=%s\n", values["type"], values["reason"], values["name"])
			log.Infof("      eventTime=%s firstTime=%s \n", values["eventTime"], values["firstTime"])
			log.Debugln("eventTime=", eventTime)
			log.Debugln("firstTime=", firstTime)
			log.Debugln("exportedTime=", exportedTime)
			err = a.addUpdateSubcode(exportedTime, eventTime, firstTime, values["name"].(string), values["reason"].(string), values["type"].(string), values["message"].(string), values["namespace"].(string))
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

func (a *appK8sRedis2Db) cnxDB() (db *sql.DB, err error) {
	nbtries := 3

	for try := 1; try <= nbtries && !a.isConnectedToDB(); try++ {
		psqlInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", a.dbHost, a.dbPort, a.dbUser, a.dbPassword, a.dbName)
		// psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", cfgEnvCnx.Host, cfgEnvCnx.Port, cfgEnvCnx.User, cfgEnvCnx.Password, cfgEnvCnx.Dbname)
		db, err = sql.Open("postgres", psqlInfo)
		if err != nil {
			log.Fatalln("Failed to connect to database")
		}
		// err = db.Ping()
		// if err != nil {
		// 	log.Fatalln("Failed to connect to database")
		// 	os.Exit(1)
		// }
	}

	return db, err
}

func (a *appK8sRedis2Db) addUpdateSubcode(exportedTime time.Time, firstTime time.Time, eventTime time.Time, eventName string, eventReason string, eventType string, eventMessage string, eventNamespace string) error {
	var err error
	if !a.isConnectedToDB() {
		cnx, err = a.cnxDB()
		if err != nil {
			log.Errorln(err.Error())
			return err
		}
	}

	sqlStatement := `
INSERT INTO k8sevents (exportedTime,firstTime,eventTime,name, reason, type,message,namespace)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err = cnx.Exec(sqlStatement, exportedTime, firstTime, eventTime, eventName, eventReason, eventType, eventMessage, eventNamespace)
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

func (a *appK8sRedis2Db) PurgeDB() error {
	var err error
	if !a.isConnectedToDB() {
		cnx, err = a.cnxDB()
		if err != nil {
			log.Errorln(err.Error())
			return err
		}
	}

	sqlStatement := fmt.Sprintf("DELETE FROM k8sevents where exportedTime <= now() - INTERVAL '%d DAYS'", a.dataRetentionInDays)
	r, err := cnx.Exec(sqlStatement)
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

func (a *appK8sRedis2Db) BackgroundPurge() {
	a.PurgeDB()
	tick := time.NewTicker(24 * time.Hour)
	for range tick.C {
		a.PurgeDB()
	}
}

func (a *appK8sRedis2Db) isConnectedToDB() bool {
	if cnx == nil {
		return false
	}
	err := cnx.Ping()
	if err != nil {
		log.Errorln("Not connected to DB")
		return false
	}
	return true
}
