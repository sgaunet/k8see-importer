// Package config provides configuration loading and validation for the k8see-importer application.
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v2"
)

// DefaultDataRetentionInDays is the default number of days to retain k8s events in the database.
const DefaultDataRetentionInDays = 30

// YamlConfig represents the yaml configuration file passed as a parameter to the program.
type YamlConfig struct {
	DbHost              string `validate:"required"                 yaml:"dbhost"`
	DbPort              string `validate:"required,gte=1,lte=65535" yaml:"dbport"`
	DbUser              string `validate:"required"                 yaml:"dbuser"`
	DbPassword          string `validate:"required"                 yaml:"dbpassword"`
	DbName              string `validate:"required"                 yaml:"dbname"`
	LogLevel            string `yaml:"loglevel"`
	RedisHost           string `validate:"required"                 yaml:"redis_host"`
	RedisPort           string `validate:"required"                 yaml:"redis_port"`
	RedisPassword       string `yaml:"redis_password"`
	RedisStream         string `validate:"required"                 yaml:"redis_stream"`
	DataRetentionInDays int    `validate:"required,gte=1"           yaml:"data_retention_in_days"`
}

// LoadConfigFromFile loads and parses the YAML configuration from the specified file.
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

// LoadConfigFromEnv loads configuration from environment variables.
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

// IsConfigValid validates the configuration using struct tags.
func (cfg *YamlConfig) IsConfigValid() error {
	validate := validator.New()
	return validate.Struct(*cfg)
	// validationErrors := err.(validator.ValidationErrors)
	// return validationErrors
}
