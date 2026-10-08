package main

import (
	"flag"
	"time"
)

const (
	maxBody           = 128 << 10
	maxWords          = 4096
	maxHeaderBytes    = 16 << 10
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 5 * time.Second
)

type config struct {
	addr      string
	staticDir string
}

func loadConfig() config {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	staticDir := flag.String("static", "../dist", "built frontend directory")
	flag.Parse()
	return config{addr: *addr, staticDir: *staticDir}
}
