package config

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"typeconverter/queue"
)

const DefaultPath = "config.json"

type Config struct {
	HTTP  HTTP  `json:"http"`
	Queue Queue `json:"queue"`
	Store Store `json:"store"`
}

type HTTP struct {
	Addr         string `json:"addr"`
	MaxBodyBytes int64  `json:"max_body_bytes"`
}

type Queue struct {
	Workers   int `json:"workers"`
	QueueSize int `json:"queue_size"`
	MaxJobs   int `json:"max_jobs"`
}

type Store struct {
	Path string `json:"path"`
}

func Default() Config {
	n := runtime.NumCPU()
	if n < 2 {
		n = 2
	}
	return Config{
		HTTP: HTTP{
			Addr:         ":8080",
			MaxBodyBytes: 10 << 20,
		},
		Queue: Queue{
			Workers:   n,
			QueueSize: 256,
			MaxJobs:   10_000,
		},
		Store: Store{
			Path: "typeconverter.db",
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.applyDefaults()
	cfg.ApplyEnv()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func LoadOrDefault(path string) (Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := Default()
		cfg.ApplyEnv()
		return cfg, nil
	}
	return Load(path)
}

func (c *Config) applyDefaults() {
	d := Default()
	if c.HTTP.Addr == "" {
		c.HTTP.Addr = d.HTTP.Addr
	}
	if c.HTTP.MaxBodyBytes <= 0 {
		c.HTTP.MaxBodyBytes = d.HTTP.MaxBodyBytes
	}
	if c.Queue.Workers <= 0 {
		c.Queue.Workers = d.Queue.Workers
	}
	if c.Queue.QueueSize <= 0 {
		c.Queue.QueueSize = d.Queue.QueueSize
	}
	if c.Queue.MaxJobs <= 0 {
		c.Queue.MaxJobs = d.Queue.MaxJobs
	}
	if c.Store.Path == "" {
		c.Store.Path = d.Store.Path
	}
}

func (c *Config) ApplyEnv() {
	if v := os.Getenv("ADDR"); v != "" {
		c.HTTP.Addr = v
	}
	if n, ok := envInt("WORKERS"); ok && n > 0 {
		c.Queue.Workers = n
	}
	if n, ok := envInt("QUEUE_SIZE"); ok && n > 0 {
		c.Queue.QueueSize = n
	}
	if n, ok := envInt("MAX_JOBS"); ok && n > 0 {
		c.Queue.MaxJobs = n
	}
	if n, ok := envInt("MAX_BODY_BYTES"); ok && n > 0 {
		c.HTTP.MaxBodyBytes = int64(n)
	}
	if v := os.Getenv("STORE"); v != "" {
		c.Store.Path = v
	}
}

func (c Config) Validate() error {
	if c.HTTP.Addr == "" {
		return fmt.Errorf("http.addr is empty")
	}
	if c.HTTP.MaxBodyBytes <= 0 {
		return fmt.Errorf("http.max_body_bytes must be > 0")
	}
	if c.Queue.Workers < 1 {
		return fmt.Errorf("queue.workers must be >= 1")
	}
	if c.Queue.QueueSize < 1 {
		return fmt.Errorf("queue.queue_size must be >= 1")
	}
	if c.Queue.MaxJobs < 1 {
		return fmt.Errorf("queue.max_jobs must be >= 1")
	}
	return nil
}

func (c Config) QueueConfig() queue.Config {
	return queue.Config{
		Workers:   c.Queue.Workers,
		QueueSize: c.Queue.QueueSize,
		MaxJobs:   c.Queue.MaxJobs,
	}
}

func envInt(name string) (int, bool) {
	v := os.Getenv(name)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}
