package config_test

import (
	"os"
	"testing"

	"github.com/sgaunet/k8see-importer/internal/config"
	"github.com/stretchr/testify/assert"
)

func clearEnv() {
	os.Setenv("DBHOST", "")
	os.Setenv("DBNAME", "")
	os.Setenv("DBPASSWORD", "")
	os.Setenv("DBPORT", "")
	os.Setenv("DBUSER", "")
	os.Setenv("REDIS_HOST", "")
	os.Setenv("REDIS_PORT", "")
	os.Setenv("REDIS_PASSWORD", "")
	os.Setenv("REDIS_STREAM", "")
	os.Setenv("LOGLEVEL", "")
	os.Setenv("DATA_RETENTION_IN_DAYS", "")
}

func TestLoadFromEnv(t *testing.T) {
	t.Parallel()
	t.Run("case with no env at all", func(t *testing.T) {
		clearEnv()
		cfg := config.LoadConfigFromEnv()
		assert.NotNil(t, cfg)
		assert.Equal(t, config.DefaultDataRetentionInDays, cfg.DataRetentionInDays)
	})

	t.Run("case with all env set", func(t *testing.T) {
		clearEnv()
		os.Setenv("DBHOST", "localhost")
		os.Setenv("DBNAME", "test")
		os.Setenv("DBPASSWORD", "password")
		os.Setenv("DBPORT", "5432")
		os.Setenv("DBUSER", "user")
		os.Setenv("REDIS_HOST", "localhost")
		os.Setenv("REDIS_PORT", "6379")
		os.Setenv("REDIS_PASSWORD", "password")
		os.Setenv("REDIS_STREAM", "stream")
		os.Setenv("LOGLEVEL", "debug")
		os.Setenv("DATA_RETENTION_IN_DAYS", "40")
		cfg := config.LoadConfigFromEnv()
		assert.NotNil(t, cfg)
		assert.Equal(t, 40, cfg.DataRetentionInDays)
		assert.Equal(t, "localhost", cfg.DbHost)
		assert.Equal(t, "test", cfg.DbName)
		assert.Equal(t, "password", cfg.DbPassword)
		assert.Equal(t, "5432", cfg.DbPort)
		assert.Equal(t, "user", cfg.DbUser)
		assert.Equal(t, "localhost", cfg.RedisHost)
		assert.Equal(t, "6379", cfg.RedisPort)
		assert.Equal(t, "password", cfg.RedisPassword)
		assert.Equal(t, "stream", cfg.RedisStream)
		assert.Equal(t, "debug", cfg.LogLevel)
	})
}

func TestIsConfigValid(t *testing.T) {
	t.Parallel()
	t.Run("case with no env at all", func(t *testing.T) {
		clearEnv()
		cfg := config.LoadConfigFromEnv()
		assert.NotNil(t, cfg)
		err := cfg.IsConfigValid()
		assert.NotNil(t, err)
	})

	t.Run("case with all env set", func(t *testing.T) {
		clearEnv()
		os.Setenv("DBHOST", "localhost")
		os.Setenv("DBNAME", "test")
		os.Setenv("DBPASSWORD", "password")
		os.Setenv("DBPORT", "5432")
		os.Setenv("DBUSER", "user")
		os.Setenv("REDIS_HOST", "localhost")
		os.Setenv("REDIS_PORT", "6379")
		os.Setenv("REDIS_PASSWORD", "password")
		os.Setenv("REDIS_STREAM", "stream")
		os.Setenv("LOGLEVEL", "debug")
		os.Setenv("DATA_RETENTION_IN_DAYS", "40")
		cfg := config.LoadConfigFromEnv()
		assert.NotNil(t, cfg)
		err := cfg.IsConfigValid()
		assert.Nil(t, err)
	})
}
