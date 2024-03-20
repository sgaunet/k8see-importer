package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v2"
)

const DefaultDataRetentionInDays = 30

// Struct representing the yaml configuration file passed as a parameter to the program
type YamlConfig struct {
	DbHost              string `yaml:"dbhost" validate:"required"`
	DbPort              string `yaml:"dbport" validate:"required,gte=1,lte=65535"`
	DbUser              string `yaml:"dbuser" validate:"required"`
	DbPassword          string `yaml:"dbpassword" validate:"required"`
	DbName              string `yaml:"dbname" validate:"required"`
	LogLevel            string `yaml:"loglevel" `
	RedisHost           string `yaml:"redis_host" validate:"required"`
	RedisPort           string `yaml:"redis_port" validate:"required"`
	RedisPassword       string `yaml:"redis_password"`
	RedisStream         string `yaml:"redis_stream" validate:"required"`
	DataRetentionInDays int    `yaml:"data_retention_in_days" validate:"required,gte=1"`
}

func LoadConfigFromFile(filename string) (*YamlConfig, error) {
	var yamlConfig YamlConfig
	yamlFile, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading YAML file: %s\n", err)
		return &yamlConfig, err
	}
	err = yaml.Unmarshal(yamlFile, &yamlConfig)
	if err != nil {
		fmt.Printf("Error parsing YAML file: %s\n", err)
		return &yamlConfig, err
	}
	return &yamlConfig, err
}

func LoadConfigFromEnv() *YamlConfig {
	var err error
	var cfg YamlConfig
	cfg.DbHost = os.Getenv("DBHOST")
	cfg.DbName = os.Getenv("DBNAME")
	cfg.DbPassword = os.Getenv("DBPASSWORD")
	cfg.DbPort = os.Getenv("DBPORT")
	cfg.DbUser = os.Getenv("DBUSER")
	cfg.RedisHost = os.Getenv("REDIS_HOST")
	cfg.RedisPort = os.Getenv("REDIS_PORT")
	cfg.RedisPassword = os.Getenv("REDIS_PASSWORD")
	cfg.RedisStream = os.Getenv("REDIS_STREAM")
	cfg.LogLevel = os.Getenv("LOGLEVEL")
	retention := os.Getenv("DATA_RETENTION_IN_DAYS")
	cfg.DataRetentionInDays, err = strconv.Atoi(retention)
	if retention == "" || err != nil {
		cfg.DataRetentionInDays = DefaultDataRetentionInDays
	}
	return &cfg
}

func (cfg *YamlConfig) IsConfigValid() error {
	validate := validator.New()
	return validate.Struct(*cfg)
	// validationErrors := err.(validator.ValidationErrors)
	// return validationErrors
}
