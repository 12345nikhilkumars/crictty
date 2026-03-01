package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/12345nikhilkumars/crictui/internal/models"
	badger "github.com/dgraph-io/badger/v4"
)

type Cache struct {
	db *badger.DB
}

func New() (*Cache, error) {
	dir := filepath.Join(os.TempDir(), "crictui-cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cache dir: %v", err)
	}

	opts := badger.DefaultOptions(dir).
		WithLoggingLevel(badger.ERROR)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("badger open: %v", err)
	}
	return &Cache{db: db}, nil
}

func (c *Cache) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

func overKey(matchID uint32, inningsID uint32) []byte {
	return []byte(fmt.Sprintf("match:%d:overs:%d", matchID, inningsID))
}

func (c *Cache) GetOvers(matchID, inningsID uint32) ([]models.OverSummary, bool) {
	var result []models.OverSummary
	err := c.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(overKey(matchID, inningsID))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &result)
		})
	})
	if err != nil {
		return nil, false
	}
	return result, true
}

func (c *Cache) PutOvers(matchID, inningsID uint32, overs []models.OverSummary) error {
	data, err := json.Marshal(overs)
	if err != nil {
		return err
	}
	return c.db.Update(func(txn *badger.Txn) error {
		return txn.Set(overKey(matchID, inningsID), data)
	})
}
