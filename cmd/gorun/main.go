package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"gorun/internal/config"
	"gorun/internal/deployer"
	"gorun/internal/handler"
	"gorun/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to YAML configuration file")
	dbPath := flag.String("db", "gorun.db", "Path to SQLite database file")
	templatesDir := flag.String("templates", "templates", "Path to HTML templates directory")
	staticDir := flag.String("static", "static", "Path to static assets directory")
	flag.Parse()

	log.Println("[INFO] Starting Gorun...")

	// Check if config file exists, if not prompt example
	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		log.Fatalf("[FATAL] Config file %q not found. Please create one based on config.example.yaml", *configPath)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("[FATAL] Failed to load configuration: %v", err)
	}
	log.Printf("[INFO] Configuration loaded (port %d, auth user %q)", cfg.Port, cfg.Username)
	if cfg.Password == "admin" {
		log.Println("[WARNING] Security Warning: Default password 'admin' is being used. Please update 'password' in your configuration file for production environments.")
	}

	s, err := store.NewStore(*dbPath)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize database store: %v", err)
	}
	defer s.Close()
	log.Printf("[INFO] SQLite store initialized at %q", *dbPath)

	d := deployer.NewDeployer(s)
	h, err := handler.NewHandler(cfg, s, d, *templatesDir)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize HTTP handler: %v", err)
	}

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Static assets handler
	if _, err := os.Stat(*staticDir); err == nil {
		fs := http.FileServer(http.Dir(*staticDir))
		mux.Handle("GET /static/", http.StripPrefix("/static/", fs))
	} else {
		log.Printf("[WARN] Static directory %q not found, static assets will not be served", *staticDir)
	}

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("[INFO] Gorun server listening on http://localhost%s (Port %d)", addr, cfg.Port)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[FATAL] Server terminated: %v", err)
	}
}
