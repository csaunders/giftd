package models

import (
	"encoding/json"

	"github.com/boltdb/bolt"
)

type Account struct {
	DatastoreName string `json:"datastore-name"`
}

type Permission struct {
}

func Save(bucket *bolt.Bucket, key string, record interface{}) error {
	if data, err := json.Marshal(record); err != nil {
		return err
	} else {
		return bucket.Put([]byte(key), data)
	}
}

func Load(bucket *bolt.Bucket, key string, record interface{}) error {
	data := bucket.Get([]byte(key))
	return json.Unmarshal(data, record)
}
