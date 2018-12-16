package storageconsul

import (
	consul "github.com/hashicorp/consul/api"
	"github.com/mholt/certmagic"
	"github.com/stretchr/testify/assert"
	"os"
	"path"
	"testing"
)

var consulClient *consul.Client

const TestPrefix = "consultlstest"

// these tests needs a running Consul server
func setupConsulEnv(t *testing.T) *ConsulStorage {

	os.Setenv(EnvNamePrefix, TestPrefix)
	os.Setenv(consul.HTTPTokenEnvName, "2f9e03f8-714b-5e4d-65ea-c983d6b172c4")

	cs, err := NewConsulStorage()
	assert.NoError(t, err)

	_, err = cs.ConsulClient.KV().DeleteTree(TestPrefix, nil)
	assert.NoError(t, err)
	return cs
}

func TestConsulStorage_Store(t *testing.T) {
	cs := setupConsulEnv(t)

	err := cs.Store(path.Join("acme", "example.com", "sites", "example.com", "example.com.crt"), []byte("crt data"))
	assert.NoError(t, err)
}

func TestConsulStorage_Exists(t *testing.T) {
	cs := setupConsulEnv(t)

	key := path.Join("acme", "example.com", "sites", "example.com", "example.com.crt")

	err := cs.Store(key, []byte("crt data"))
	assert.NoError(t, err)

	exists := cs.Exists(key)
	assert.True(t, exists)
}

func TestConsulStorage_Load(t *testing.T) {
	cs := setupConsulEnv(t)

	key := path.Join("acme", "example.com", "sites", "example.com", "example.com.crt")
	content := []byte("crt data")

	err := cs.Store(key, content)
	assert.NoError(t, err)

	contentLoded, err := cs.Load(key)
	assert.NoError(t, err)

	assert.Equal(t, content, contentLoded)
}

func TestConsulStorage_Delete(t *testing.T) {
	cs := setupConsulEnv(t)

	key := path.Join("acme", "example.com", "sites", "example.com", "example.com.crt")
	content := []byte("crt data")

	err := cs.Store(key, content)
	assert.NoError(t, err)

	err = cs.Delete(key)
	assert.NoError(t, err)

	exists := cs.Exists(key)
	assert.False(t, exists)

	contentLoaded, err := cs.Load(key)
	assert.Nil(t, contentLoaded)

	_, ok := err.(certmagic.ErrNotExist)
	assert.True(t, ok)
}

func TestConsulStorage_Stat(t *testing.T) {
	cs := setupConsulEnv(t)

	key := path.Join("acme", "example.com", "sites", "example.com", "example.com.crt")
	content := []byte("crt data")

	err := cs.Store(key, content)
	assert.NoError(t, err)

	info, err := cs.Stat(key)
	assert.NoError(t, err)

	assert.Equal(t, key, info.Key)
}

func TestConsulStorage_List(t *testing.T) {
	cs := setupConsulEnv(t)

	err := cs.Store(path.Join("acme", "example.com", "sites", "example.com", "example.com.crt"), []byte("crt"))
	assert.NoError(t, err)
	err = cs.Store(path.Join("acme", "example.com", "sites", "example.com", "example.com.key"), []byte("key"))
	assert.NoError(t, err)
	err = cs.Store(path.Join("acme", "example.com", "sites", "example.com", "example.com.json"), []byte("meta"))
	assert.NoError(t, err)

	keys, err := cs.List(path.Join("acme", "example.com", "sites", "example.com"), true)
	assert.NoError(t, err)
	assert.Len(t, keys, 3)
	assert.Contains(t, keys, path.Join("acme", "example.com", "sites", "example.com", "example.com.crt"))
}

func TestConsulStorage_ListNonRecursive(t *testing.T) {
	cs := setupConsulEnv(t)

	err := cs.Store(path.Join("acme", "example.com", "sites", "example.com", "example.com.crt"), []byte("crt"))
	assert.NoError(t, err)
	err = cs.Store(path.Join("acme", "example.com", "sites", "example.com", "example.com.key"), []byte("key"))
	assert.NoError(t, err)
	err = cs.Store(path.Join("acme", "example.com", "sites", "example.com", "example.com.json"), []byte("meta"))
	assert.NoError(t, err)

	keys, err := cs.List(path.Join("acme", "example.com", "sites"), false)
	assert.NoError(t, err)

	assert.Len(t, keys, 1)
	assert.Contains(t, keys, path.Join("acme", "example.com", "sites", "example.com"))
}


func TestConsulStorage_TryLockUnlock(t *testing.T) {
	cs := setupConsulEnv(t)
	lockKey := path.Join("acme", "example.com", "sites", "example.com", "lock")

	waiter, err := cs.TryLock(lockKey)
	assert.NoError(t, err)
	assert.Nil(t, waiter)

	err = cs.Unlock(lockKey)
	assert.NoError(t, err)
}

func TestConsulStorage_TryLockLock(t *testing.T) {
	cs := setupConsulEnv(t)
	cs2 := setupConsulEnv(t)
	lockKey := path.Join("acme", "example.com", "sites", "example.com", "lock")

	waiter, err := cs.TryLock(lockKey)
	assert.NoError(t, err)
	assert.Nil(t, waiter)

	waiter, err = cs2.TryLock(lockKey)
	assert.NoError(t, err)
	assert.NotNil(t, waiter)

	err = cs.Unlock(lockKey)
	assert.NoError(t, err)

	waiter.Wait()

	waiter, err = cs2.TryLock(lockKey)
	assert.NoError(t, err)
	assert.Nil(t, waiter)

	err = cs2.Unlock(lockKey)
	assert.NoError(t, err)
}
