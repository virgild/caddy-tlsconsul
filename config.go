package storageconsul

import (
	"time"
)

const (
	// DefaultPrefix defines the default prefix in KV store
	DefaultPrefix = "caddytls"

	// DefaultAESKey needs to be 32 bytes long
	DefaultAESKey = "consultls-1234567890-caddytls-32"

	// DefaultValuePrefix sets a prefix to KV values to check validation
	DefaultValuePrefix = "caddy-storage-consul"

	// DefaultTimeout is the default timeout for Consul connections
	DefaultTimeout = 10 * time.Second

	// EnvNameAESKey defines the env variable name to override AES key
	EnvNameAESKey = "CADDY_CLUSTERING_CONSUL_AESKEY"

	// EnvNamePrefix defines the env variable name to override KV key prefix
	EnvNamePrefix = "CADDY_CLUSTERING_CONSUL_PREFIX"

	// EnvValuePrefix defines the env variable name to override KV value prefix
	EnvValuePrefix = "CADDY_CLUSTERING_CONSUL_VALUEPREFIX"
)

type Config struct {
	ConsulAddr        string
	AESKey            []byte
	ValuePrefix       string
	Prefix            string
	ConsulToken       string
	Timeout           time.Duration
	ConsulTls         bool
	ConsulTlsInsecure bool
}

type Option func(c *Config)

func WithConsulAddr(consulAddr string) Option {
	return func(c *Config) {
		c.ConsulAddr = consulAddr
	}
}

func WithAESKey(aesKey string) Option {
	return func(c *Config) {
		c.AESKey = []byte(aesKey)
	}
}

func WithPrefix(prefix string) Option {
	return func(c *Config) {
		c.Prefix = prefix
	}
}

func WithValuePrefix(valuePrefix string) Option {
	return func(c *Config) {
		c.ValuePrefix = valuePrefix
	}
}

func WithConsulToken(consulToken string) Option {
	return func(c *Config) {
		c.ConsulToken = consulToken
	}
}

func WithConsulTls(enable bool) Option {
	return func(c *Config) {
		c.ConsulTls = true
	}
}

func WithConsulTlsInsecure(enable bool) Option {
	return func(c *Config) {
		c.ConsulTlsInsecure = enable
	}
}

func WithTimeout(dur time.Duration) Option {
	return func(c *Config) {
		c.Timeout = dur
	}
}
