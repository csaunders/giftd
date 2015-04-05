package middleware

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/csaunders/giftd/models"
	"github.com/zenazn/goji/web"
)

const Datastore string = "datastore"
const accountDetails string = "account-details"

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
		delete(cache, name)
	})
}

func loadDatastore(a interface{}) (*store, error) {
	account, ok := a.(models.Account)
	if !ok {
		return nil, errors.New("Invalid Account Information")
	}
	var datastore *store
	var err error
	synchronized(func() {
		datastore := cache[account.DatastoreName()]
		if datastore == nil {
			db, err := bolt.Open(account.DatastoreName(), 0600, &bolt.Options{Timeout: 1 * time.Second})
			if err == nil {
				datastore = &store{Db: db, Wg: new(sync.WaitGroup)}
				datastore.Wg.Add(1)
				go datastoreCloser(account.DatastoreName(), datastore)
			}
		}
	})

	return datastore, err
}

func unloadDatastore(datastore *store) {
	datastore.Wg.Done()
}

func DatastoreLoader(c *web.C, h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		datastore, err := loadDatastore(c.Env[accountDetails])
		if err == nil {
			defer unloadDatastore(datastore)
			c.Env[Datastore] = datastore.Db

			h.ServeHTTP(w, r)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("It's not you, it's us."))
		}
	}
	return http.HandlerFunc(fn)
}
