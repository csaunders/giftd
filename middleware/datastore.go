package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/csaunders/giftd/models"
	"github.com/zenazn/goji/web"
)

const Datastore string = "datastore"
const AccountDetails string = "account-details"
const AccountId string = "account_id"

const sels string = "[0-9a-f]"

var pattern string = fmt.Sprintf("%s{8}-%s{4}-%s{4}-%s{4}-%s{12}", sels, sels, sels, sels, sels)

var pathPattern *regexp.Regexp = regexp.MustCompile(pattern)

var cache map[string]*store = map[string]*store{}

var storeMutex *sync.Mutex = new(sync.Mutex)

func synchronized(fn func()) {
	storeMutex.Lock()
	defer storeMutex.Unlock()
	fn()
}

type store struct {
	Db *bolt.DB
	Wg *sync.WaitGroup
}

func datastoreCloser(name string, datastore *store) {
	datastore.Wg.Wait()
	synchronized(func() {
		datastore.Db.Close()
		if cache[name] != nil {
			delete(cache, name)
		}
	})
}

func openDatastore(name string) (*store, error) {
	var datastore *store
	var err error
	synchronized(func() {
		datastore = cache[name]
		if datastore == nil {
			db, err := bolt.Open(name, 0600, &bolt.Options{Timeout: 1 * time.Second})
			if err == nil {
				datastore = &store{}
				datastore.Db = db
				datastore.Wg = new(sync.WaitGroup)
				datastore.Wg.Add(1)
				cache[name] = datastore
				go datastoreCloser(name, datastore)
			}
		} else {
			datastore.Wg.Add(1)
		}
	})

	return datastore, err
}

func datastoreNameFromEnv(c *web.C) (string, error) {
	account, ok := c.Env[AccountDetails].(models.Account)
	if !ok {
		return "", errors.New("No Account Information")
	}
	return account.DatastoreName(), nil
}

func datastoreNameFromPath(c *web.C, path string) (string, error) {
	var account models.Account
	var db *bolt.DB
	var ok bool
	if db, ok = c.Env[ConfigurationDB].(*bolt.DB); !ok {
		return "", errors.New("Cannot load configuration database")
	}
	err := db.View(func(tx *bolt.Tx) error {
		idsBucket, err := models.ApiClientIdsBucket(tx)
		if err != nil {
			return err
		}

		uuids := pathPattern.FindAllString(path, -1)
		if uuids == nil {
			return errors.New("Cannot load database for path")
		}

		var token []byte
		for _, uuid := range uuids {
			if token = idsBucket.Get([]byte(uuid)); token != nil {
				break
			}
		}

		if token == nil {
			return models.RecordNotFound
		}

		clientsBucket, err := models.ApiClientsBucket(tx)
		if err != nil {
			return err
		}

		return models.Load(clientsBucket, string(token), &account)
	})

	return account.DatastoreName(), err
}

func loadDatastore(c *web.C, r *http.Request) (*store, error) {
	var name string
	var err error
	if name, err = datastoreNameFromEnv(c); err != nil {
		if name, err = datastoreNameFromPath(c, r.URL.Path); err != nil {
			return nil, errors.New("loadDatastore: could not find datastore")
		}
	}
	return openDatastore(name)
}

func unloadDatastore(datastore *store) {
	datastore.Wg.Done()
}

func DatastoreLoader(c *web.C, h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		datastore, err := loadDatastore(c, r)
		if err == nil {
			defer unloadDatastore(datastore)
			c.Env[Datastore] = datastore.Db

			h.ServeHTTP(w, r)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("It's not you, it's us."))
			w.Write([]byte(err.Error()))
		}
	}
	return http.HandlerFunc(fn)
}

type DbHandler func(db *bolt.DB, c web.C, w http.ResponseWriter, r *http.Request)
type Initializer func(db *bolt.DB) error
type DatabaseProvider func(init Initializer, handler DbHandler) func(c web.C, w http.ResponseWriter, r *http.Request)

func EnvironmentDatabaseProvider(init Initializer, handler DbHandler) func(c web.C, w http.ResponseWriter, r *http.Request) {
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		var err error
		db, ok := c.Env[Datastore].(*bolt.DB)
		if ok {
			err = init(db)
		}

		if err != nil || !ok {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Datastore Unavailable"))
			return
		}
		handler(db, c, w, r)
	}
}
