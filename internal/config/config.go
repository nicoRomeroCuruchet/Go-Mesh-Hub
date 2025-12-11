package config

import "flag"

type Config struct {
	LocalPort int
	WebPort   int 
	TunIP     string
	Secret    string
	ExitNodeIP string 
}

func Load() *Config {
	cfg := &Config{}
	flag.IntVar(&cfg.LocalPort, "local-port", 45678, "Local UDP port to listen on")
	flag.IntVar(&cfg.WebPort, "web-port", 8080, "TCP port for Web Dashboard") 
	flag.StringVar(&cfg.TunIP, "tun-ip", "10.0.0.1", "Virtual IP of this Hub")
	flag.StringVar(&cfg.Secret, "secret", "change-this-password", "Shared secret for encryption")
	flag.StringVar(&cfg.ExitNodeIP, "exit-node", "", "Virtual IP of the peer acting as Exit Node")
	flag.Parse()
	return cfg
}