package main

import (
	"fmt"
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

// Struct representing the yaml configuration file passed as a parameter to the program
type YamlConfig struct {
	DbHost        string `yaml:"dbhost"`
	DbPort        string `yaml:"dbport"`
	DbUser        string `yaml:"dbuser"`
	DbPassword    string `yaml:"dbpassword"`
	DbName        string `yaml:"dbname"`
	LogLevel      string `yaml:"loglevel"`
	RedisHost     string `yaml:"redis_host"`
	RedisPort     string `yaml:"redis_port"`
	RedisPassword string `yaml:"redis_password"`
	RedisStream   string `yaml:"redis_stream"`
}

func ReadyamlConfigFile(filename string) (YamlConfig, error) {
	var yamlConfig YamlConfig

	yamlFile, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading YAML file: %s\n", err)
		return yamlConfig, err
	}

	err = yaml.Unmarshal(yamlFile, &yamlConfig)
	if err != nil {
		fmt.Printf("Error parsing YAML file: %s\n", err)
		return yamlConfig, err
	}

	return yamlConfig, err
}
