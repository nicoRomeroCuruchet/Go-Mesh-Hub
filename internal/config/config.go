package config

import "flag"

type Config struct {
	LocalPort int
	WebPort   int // New field
	TunIP     string
	Secret    string
}

func Load() *Config {
	cfg := &Config{}
	flag.IntVar(&cfg.LocalPort, "local-port", 45678, "Local UDP port to listen on")
	flag.IntVar(&cfg.WebPort, "web-port", 8080, "TCP port for Web Dashboard") // Default 8080
	flag.StringVar(&cfg.TunIP, "tun-ip", "10.0.0.1", "Virtual IP of this Hub")
	flag.StringVar(&cfg.Secret, "secret", "change-this-password", "Shared secret for encryption")
	flag.Parse()
	return cfg
}