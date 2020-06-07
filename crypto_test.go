package storageconsul

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestConsulStorage_EncryptDecryptStorageData(t *testing.T) {
	cs := New(WithValuePrefix(DefaultPrefix), WithAESKey(DefaultAESKey))

	testDate := time.Now()

	sd := &StorageData{
		Value:    []byte("crt data"),
		Modified: testDate,
	}

	encryptedData, err := cs.EncryptStorageData(sd)
	assert.NoError(t, err)

	decryptedData, err := cs.DecryptStorageData(encryptedData)
	assert.NoError(t, err)

	assert.Equal(t, sd.Value, decryptedData.Value)
	assert.Equal(t, sd.Modified.Format(time.RFC822), decryptedData.Modified.Format(time.RFC822))
}
