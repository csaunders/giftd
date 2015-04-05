package models

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"

	"github.com/boltdb/bolt"
)

const defaultDatastore string = "giftd.db"
const accessTokenSize int = 32

type set map[string]bool

func (s set) add(items []string) set {
	for _, i := range items {
		s[i] = true
	}
	return s
}

func (s set) remove(items []string) set {
	for _, i := range items {
		if s[i] {
			delete(s, i)
		}
	}
	return s
}

func (s set) has(i string) bool {
	return s[i]
}

func (s set) array() []string {
	ary := make([]string, len(s))
	idx := 0
	for key, _ := range s {
		ary[idx] = key
		idx++
	}
	return ary
}

type Account struct {
	Token       string   `json:"access-token"`
	Datastore   string   `json:"datastore"`
	Permissions []string `json:"permissions"`
}

func NewAccount() (*Account, error) {
	token, err := generateToken(accessTokenSize)
	if err != nil {
		return nil, err
	}
	datastore, err := generateDatastoreName(accessTokenSize)
	if err != nil {
		return nil, err
	}
	return &Account{Token: token, Datastore: datastore}, nil
}

func (a *Account) DatastoreName() string {
	if len(a.Datastore) <= 0 {
		return defaultDatastore
	}
	return a.Datastore
}

func (a *Account) AddPermissions(perms []string) {
	set := make(set).add(a.Permissions).add(perms)
	a.Permissions = set.array()
}

func (a *Account) RemovePermissions(perms []string) {
	set := make(set).add(a.Permissions).remove(perms)
	a.Permissions = set.array()
}

func (a *Account) HasPermission(perm string) bool {
	set := make(set).add(a.Permissions)
	return set.has(perm)
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

func generateToken(size int) (string, error) {
	rb := make([]byte, size)
	_, err := rand.Read(rb)

	if err != nil {
		return "", err
	}
	return hex.EncodeToString(rb), nil
}

func generateDatastoreName(size int) (string, error) {
	name, err := generateToken(size)
	if err != nil {
		return "", err
	}
	return name + ".db", nil
}
