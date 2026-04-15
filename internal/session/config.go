package session

import "time"

const ProtocolVersion uint16 = 2

type Config struct {
	Name              string
	PSK               []byte
	Version           uint16
	HandshakeTimeout  time.Duration
	HeartbeatInterval time.Duration
	IdleTimeout       time.Duration
	MaxMessageSize    int
}

func (c Config) withDefaults() Config {
	if c.Version == 0 {
		c.Version = ProtocolVersion
	}
	if c.HandshakeTimeout == 0 {
		c.HandshakeTimeout = 10 * time.Second
	}
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = 15 * time.Second
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = 45 * time.Second
	}
	if c.MaxMessageSize == 0 {
		c.MaxMessageSize = 4 * 1024
	}
	return c
}
