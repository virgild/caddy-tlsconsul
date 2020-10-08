package storageconsul

import (
	"context"
	"fmt"
	"net"
	"path"
	"strings"
	"time"

	"github.com/caddyserver/certmagic"
	consul "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
)

// Storage holds all parameters for the Consul connection
type Storage struct {
	certmagic.Storage
	ConsulClient *consul.Client
	logger       *zap.SugaredLogger
	locks        map[string]*consul.Lock

	Address     string `json:"address"`
	Token       string `json:"token"`
	Timeout     int    `json:"timeout"`
	Prefix      string `json:"prefix"`
	ValuePrefix string `json:"value_prefix"`
	AESKey      []byte `json:"aes_key"`
	TlsEnabled  bool   `json:"tls_enabled"`
	TlsInsecure bool   `json:"tls_insecure"`
}

// New connects to Consul and returns a Storage
func New() *Storage {
	// create Storage and pre-set values
	s := Storage{
		locks:       make(map[string]*consul.Lock),
		AESKey:      []byte(DefaultAESKey),
		ValuePrefix: DefaultValuePrefix,
		Prefix:      DefaultPrefix,
		Timeout:     DefaultTimeout,
	}

	return &s
}

// helper function to prefix key
func (s *Storage) prefixKey(key string) string {
	return path.Join(s.Prefix, key)
}

// Lock acquires a lock for the given key or blocks until it gets it
func (s Storage) Lock(ctx context.Context, key string) error {
	// if we already hold the lock, return early
	if _, exists := s.locks[key]; exists {
		return nil
	}

	// prepare the lock
	lock, err := s.ConsulClient.LockKey(s.prefixKey(key))
	if err != nil {
		return fmt.Errorf("could not create lock for %s: %w", s.prefixKey(key), err)
	}

	// acquire the lock and return a channel that is closed upon lost
	lockActive, err := lock.Lock(ctx.Done())
	if err != nil {
		return fmt.Errorf("could not get lock for %s: %w", s.prefixKey(key), err)
	}

	// auto-unlock and clean list of locks in case of lost
	go func() {
		<-lockActive
		s.Unlock(key)
	}()

	// save the lock
	s.locks[key] = lock

	return nil
}

// Unlock releases a specific lock
func (s Storage) Unlock(key string) error {
	// check if we own it and unlock
	lock, exists := s.locks[key]
	if !exists {
		return fmt.Errorf("lock %s not found", key)
	}

	err := lock.Unlock()
	if err != nil {
		return fmt.Errorf("unable to unlock %s: %w", key, err)
	}

	delete(s.locks, key)
	return nil
}

// Store saves encrypted value at key in Consul KV
func (s Storage) Store(key string, value []byte) error {
	kv := &consul.KVPair{Key: s.prefixKey(key)}

	// prepare the stored data
	consulData := &StorageData{
		Value:    value,
		Modified: time.Now(),
	}

	encryptedValue, err := s.EncryptStorageData(consulData)
	if err != nil {
		return fmt.Errorf("unable to encode data for %v: %w", key, err)
	}

	kv.Value = encryptedValue

	if _, err = s.ConsulClient.KV().Put(kv, nil); err != nil {
		return fmt.Errorf("unable to store data for %v: %w", key, err)
	}

	return nil
}

// Load retrieves the value for key from Consul KV
func (s Storage) Load(key string) ([]byte, error) {
	kv, _, err := s.ConsulClient.KV().Get(s.prefixKey(key), &consul.QueryOptions{RequireConsistent: true})
	if err != nil {
		return nil, fmt.Errorf("unable to obtain data for %s: %w", key, err)
	} else if kv == nil {
		return nil, certmagic.ErrNotExist(fmt.Errorf("key %s does not exist", key))
	}

	contents, err := s.DecryptStorageData(kv.Value)
	if err != nil {
		return nil, fmt.Errorf("unable to decrypt data for %s: %w", key, err)
	}

	return contents.Value, nil
}

// Delete a key
func (s Storage) Delete(key string) error {
	// first obtain existing keypair
	kv, _, err := s.ConsulClient.KV().Get(s.prefixKey(key), &consul.QueryOptions{RequireConsistent: true})
	if err != nil {
		return fmt.Errorf("unable to obtain data for %s: %w", key, err)
	} else if kv == nil {
		return certmagic.ErrNotExist(err)
	}

	// no do a Check-And-Set operation to verify we really deleted the key
	if success, _, err := s.ConsulClient.KV().DeleteCAS(kv, nil); err != nil {
		return fmt.Errorf("unable to delete data for %s: %w", key, err)
	} else if !success {
		return fmt.Errorf("failed to lock data delete for %s", key)
	}

	return nil
}

// Exists checks if a key exists
func (s Storage) Exists(key string) bool {
	kv, _, err := s.ConsulClient.KV().Get(s.prefixKey(key), &consul.QueryOptions{RequireConsistent: true})
	if kv != nil && err == nil {
		return true
	}
	return false
}

// List returns a list with all keys under a given prefix
func (s Storage) List(prefix string, recursive bool) ([]string, error) {
	var keysFound []string

	// get a list of all keys at prefix
	keys, _, err := s.ConsulClient.KV().Keys(s.prefixKey(prefix), "", &consul.QueryOptions{RequireConsistent: true})
	if err != nil {
		return keysFound, err
	}

	if len(keys) == 0 {
		return keysFound, certmagic.ErrNotExist(fmt.Errorf("no keys at %s", prefix))
	}

	// remove default prefix from keys
	for _, key := range keys {
		if strings.HasPrefix(key, s.prefixKey(prefix)) {
			key = strings.TrimPrefix(key, s.Prefix+"/")
			keysFound = append(keysFound, key)
		}
	}

	// if recursive wanted, just return all keys
	if recursive {
		return keysFound, nil
	}

	// for non-recursive split path and look for unique keys just under given prefix
	keysMap := make(map[string]bool)
	for _, key := range keysFound {
		dir := strings.Split(strings.TrimPrefix(key, prefix+"/"), "/")
		keysMap[dir[0]] = true
	}

	keysFound = make([]string, 0)
	for key := range keysMap {
		keysFound = append(keysFound, path.Join(prefix, key))
	}

	return keysFound, nil
}

// Stat returns statistic data of a key
func (s Storage) Stat(key string) (certmagic.KeyInfo, error) {
	kv, _, err := s.ConsulClient.KV().Get(s.prefixKey(key), &consul.QueryOptions{RequireConsistent: true})
	if err != nil {
		return certmagic.KeyInfo{}, fmt.Errorf("unable to obtain data for %s: %w", key, err)
	} else if kv == nil {
		return certmagic.KeyInfo{}, certmagic.ErrNotExist(fmt.Errorf("key %s does not exist", key))
	}

	contents, err := s.DecryptStorageData(kv.Value)
	if err != nil {
		return certmagic.KeyInfo{}, fmt.Errorf("unable to decrypt data for %s: %w", key, err)
	}

	return certmagic.KeyInfo{
		Key:        key,
		Modified:   contents.Modified,
		Size:       int64(len(contents.Value)),
		IsTerminal: false,
	}, nil
}

func (s *Storage) createConsulClient() error {
	// get the default config
	consulCfg := consul.DefaultConfig()
	if s.Address != "" {
		consulCfg.Address = s.Address
	}
	if s.Token != "" {
		consulCfg.Token = s.Token
	}
	if s.TlsEnabled {
		consulCfg.Scheme = "https"
	}
	consulCfg.TLSConfig.InsecureSkipVerify = s.TlsInsecure

	// set a dial context to prevent default keepalive
	consulCfg.Transport.DialContext = (&net.Dialer{
		Timeout:   time.Duration(s.Timeout) * time.Second,
		KeepAlive: time.Duration(s.Timeout) * time.Second,
	}).DialContext

	// create the Consul API client
	consulClient, err := consul.NewClient(consulCfg)
	if err != nil {
		return fmt.Errorf("unable to create Consul client: %w", err)
	}
	if _, err := consulClient.Agent().NodeName(); err != nil {
		return fmt.Errorf("unable to ping Consul: %w", err)
	}

	s.ConsulClient = consulClient
	return nil
}
