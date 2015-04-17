package models

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/boltdb/bolt"
)

const defaultDatastore string = "giftd.db"
const accessTokenSize int = 32

var RecordNotFound error = errors.New("record does not exist")

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
	Id          string   `json:"id"`
	Token       string   `json:"access-token"`
	Datastore   string   `json:"datastore"`
	Permissions []string `json:"permissions"`
}

func NewAccount() (*Account, error) {
	var err error
	account := &Account{}
	if account.Id, err = GenUUID(); err != nil {
		return nil, err
	}
	if account.Token, err = generateToken(accessTokenSize); err != nil {
		return nil, err
	}
	if account.Datastore, err = generateDatastoreName(accessTokenSize); err != nil {
		return nil, err
	}
	return account, nil
}

func (a *Account) DatastoreName() string {
	if len(a.Datastore) <= 0 {
		return defaultDatastore
	}
	return a.Datastore
}

func (a *Account) SetPermissions(perms []string) {
	if len(perms) > 0 {
		a.Permissions = perms
	}
}

func (a *Account) SetDatastore(datastore string) {
	if len(datastore) > 0 {
		a.Datastore = datastore
	}
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
	var data []byte
	var err error
	if data, err = LoadRaw(bucket, key); err != nil {
		return err
	}
	return json.Unmarshal(data, record)
}

func LoadRaw(bucket *bolt.Bucket, key string) ([]byte, error) {
	data := bucket.Get([]byte(key))
	if data == nil {
		return []byte{}, RecordNotFound
	}
	return data, nil
}

func GenUUID() (string, error) {
	urandom, err := os.OpenFile("/dev/urandom", os.O_RDONLY, 0)
	if err != nil {
		return "", err
	}
	defer urandom.Close()
	b := make([]byte, 16)
	n, err := urandom.Read(b)

	if err != nil {
		return "", err
	} else if n != len(b) {
		return "", errors.New("Could not read a sufficient number of bytes")
	}
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	return uuid, nil
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
