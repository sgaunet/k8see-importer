package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	//v1 "k8s.io/api/events/v1"
	"github.com/go-redis/redis/v7"
	//"github.com/gomodule/redigo/redis"
	"database/sql"

	_ "github.com/lib/pq"
	"github.com/robinjoseph08/redisqueue/v2"
	"github.com/sirupsen/logrus"
)

type k8sEvent struct {
	// e.FirstTimestamp, e.Type, e.Reason, e.Name, e.Message, e.UID
	Type    string `json: "type"`
	Reason  string `json: "reason"`
	Name    string `json: "name"`
	Message string `json: "message"`
}

type appK8sRedis2Db struct {
	redisHost     string
	redisPort     string
	redisPassword string
	redisStream   string
	dbHost        string
	dbPort        string
	dbUser        string
	dbPassword    string
	dbName        string
	consumer      *redisqueue.Consumer
}

var log = logrus.New()
var cnx *sql.DB

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
	var fileConfigName string
	flag.StringVar(&fileConfigName, "f", "", "YAML file to parse.")
	flag.Parse()

	if fileConfigName == "" {
		log.Fatal("No config file specified.")
		os.Exit(1)
	}

	cfg, err := ReadyamlConfigFile(fileConfigName)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	initTrace(cfg.LogLevel)
	app := NewApp(cfg.RedisHost, cfg.RedisPort, cfg.RedisPassword, cfg.RedisStream, cfg.DbHost, cfg.DbPort, cfg.DbUser, cfg.DbName, cfg.DbPassword)
	app.redis2PG()
}

func NewApp(redisHost string, redisPort string, redisPassword string, redisStream string, dbHost string, dbPort string, dbUser string, dbName string, dbPassword string) *appK8sRedis2Db {
	app := appK8sRedis2Db{
		redisHost:     redisHost,
		redisPort:     redisPort,
		redisPassword: redisPassword,
		redisStream:   redisStream,
		dbHost:        dbHost,
		dbPort:        dbPort,
		dbName:        dbName,
		dbUser:        dbUser,
		dbPassword:    dbPassword,
	}

	app.InitConsumer()
	return &app
}

func (a *appK8sRedis2Db) InitConsumer() {
	var err error
	addr := fmt.Sprintf("%s:%s", a.redisHost, a.redisPort)
	a.consumer, err = redisqueue.NewConsumerWithOptions(&redisqueue.ConsumerOptions{
		//Name:              "localhost",
		VisibilityTimeout: 60 * time.Second,
		BlockingTimeout:   5 * time.Second,
		ReclaimInterval:   1 * time.Second,
		BufferSize:        100,
		Concurrency:       10,
		RedisOptions: &redis.Options{
			Addr:     addr,
			Password: a.redisPassword, // no password set
			DB:       0,               // use default DB
		},
	})
	if err != nil {
		log.Fatalln(err.Error())
		os.Exit(1)
	}
}

func (a *appK8sRedis2Db) redis2PG() {
	var err error
	a.consumer.Register(a.redisStream, a.process)
	cnx, err = a.cnxDB()
	if err != nil {
		log.Fatalln(err.Error())
		os.Exit(1)
	}

	go func() {
		for err := range a.consumer.Errors {
			// handle errors accordingly
			log.Errorf("err: %+v\n", err)
		}
	}()
	a.consumer.Run()
	log.Infoln("Stop consumer")
}

func (a *appK8sRedis2Db) process(msg *redisqueue.Message) error {
	log.Infof("NEW=> type=%s reason=%s name=%s\n", msg.Values["type"], msg.Values["reason"], msg.Values["name"])
	a.addUpdateSubcode(time.Now(), msg.Values["name"].(string), msg.Values["reason"].(string), msg.Values["type"].(string), msg.Values["message"].(string))
	return nil
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

func (a *appK8sRedis2Db) addUpdateSubcode(eventDate time.Time, eventName string, eventReason string, eventType string, eventMessage string) error {
	var err error
	if !a.isConnectedToDB() {
		cnx, err = a.cnxDB()
		if err != nil {
			log.Errorln(err.Error())
			return err
		}
	}
	sqlStatement := `
INSERT INTO k8sevents (date,name, reason, type,message)
VALUES ($1, $2, $3,$4,$5)`
	_, err = cnx.Exec(sqlStatement, eventDate, eventName, eventReason, eventType, eventMessage)
	if err != nil {
		log.Errorf("Insert failed : %s\n", err.Error())
		return err
	}
	return err
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
