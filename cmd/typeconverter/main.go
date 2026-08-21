package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"typeconverter/api"
	"typeconverter/config"
)

func main() {
	configPath := flag.String("config", config.DefaultPath, "path to JSON config file")
	flag.Parse()

	path := *configPath
	if v := os.Getenv("CONFIG"); v != "" && path == config.DefaultPath {
		path = v
	}

	cfg, err := loadConfig(path, *configPath != config.DefaultPath || os.Getenv("CONFIG") != "")
	if err != nil {
		log.Fatal(err)
	}

	srv := api.NewFromConfig(cfg)
	defer srv.Close()

	log.Printf("listening on %s workers=%d queue=%d max_body=%d max_jobs=%d store=%s config=%s",
		cfg.HTTP.Addr, cfg.Queue.Workers, cfg.Queue.QueueSize, cfg.HTTP.MaxBodyBytes, cfg.Queue.MaxJobs, cfg.Store.Path, path)
	log.Fatal(http.ListenAndServe(cfg.HTTP.Addr, srv.Handler()))
}

func loadConfig(path string, required bool) (config.Config, error) {
	if required {
		return config.Load(path)
	}
	return config.LoadOrDefault(path)
}
