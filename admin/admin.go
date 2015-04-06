package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/boltdb/bolt"
	"github.com/csaunders/giftd/middleware"
	"github.com/csaunders/giftd/models"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
)

type TokenOptions struct {
	AccessToken string
	Permissions string
}

func unavailable(err error, w http.ResponseWriter) error {
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte(err.Error()))
	return err
}

func notFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	body, _ := json.Marshal(struct {
		Err string `json:"error"`
	}{"Resource Not Found"})
	w.Write(body)
}

func invalid(err error, w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotAcceptable)
	body, _ := json.Marshal(struct {
		Err string `json:"error"`
	}{err.Error()})
	w.Write(body)
}

func retrieveDb(c web.C, w http.ResponseWriter) (*bolt.DB, error) {
	if db, ok := c.Env[middleware.ConfigurationDB].(*bolt.DB); ok {
		return db, nil
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte("retrieveDb: database not available in context"))
	return nil, errors.New("retrieveDb: database not available in context")
}

func findClient(db *bolt.DB, c web.C, raw bool) (models.Account, error, []byte) {
	var account models.Account
	var rawData []byte
	err := db.View(func(tx *bolt.Tx) error {
		token := c.URLParams["token"]
		bucket, err := models.ApiClientsBucket(tx)
		if err != nil {
			return err
		}
		if raw {
			rawData, err = models.LoadRaw(bucket, token)
		} else {
			err = models.Load(bucket, token, &account)
		}
		return err
	})
	return account, err, rawData
}

func modifyPermissions(db *bolt.DB, r io.Reader, account *models.Account, operation func([]string)) error {
	var perms struct {
		Permissions []string `json:"permissions"`
	}
	err := json.NewDecoder(r).Decode(&perms)
	if err != nil {
		return err
	}
	operation(perms.Permissions)
	return db.Update(func(tx *bolt.Tx) error {
		bucket, err := models.ApiClientsBucket(tx)
		if err != nil {
			return err
		}
		return models.Save(bucket, account.Token, account)
	})
}

func listClients(c web.C, w http.ResponseWriter, r *http.Request) {
	db, err := retrieveDb(c, w)
	if err != nil {
		return
	}
	var clientTokens []string
	err = db.View(func(tx *bolt.Tx) error {
		clients, err := models.ApiClientsBucket(tx)
		if err != nil {
			return unavailable(err, w)
		}
		clients.ForEach(func(key, value []byte) error {
			clientTokens = append(clientTokens, string(key))
			return nil
		})
		return nil
	})
	if err == nil {
		data, _ := json.Marshal(struct {
			Tokens []string `json:"tokens"`
		}{clientTokens})
		w.Write(data)
	}
}

func showClient(c web.C, w http.ResponseWriter, r *http.Request) {
	db, err := retrieveDb(c, w)
	if err != nil {
		return
	}
	_, err, clientAccount := findClient(db, c, true)
	if err != nil {
		unavailable(err, w)
		return
	}
	w.Write(clientAccount)
}

func addPermissions(c web.C, w http.ResponseWriter, r *http.Request) {
	db, err := retrieveDb(c, w)
	if err != nil {
		return
	}
	clientAccount, err, _ := findClient(db, c, false)
	if err == nil {
		err = modifyPermissions(db, r.Body, &clientAccount, (&clientAccount).AddPermissions)
	}
	switch err {
	case nil:
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(""))
	case models.RecordNotFound:
		notFound(w)
	default:
		unavailable(err, w)
	}
}

func removePermissions(c web.C, w http.ResponseWriter, r *http.Request) {
	db, err := retrieveDb(c, w)
	if err != nil {
		return
	}
	clientAccount, err, _ := findClient(db, c, false)
	if err == nil {
		err = modifyPermissions(db, r.Body, &clientAccount, (&clientAccount).RemovePermissions)
	}
	switch err {
	case nil:
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(""))
	case models.RecordNotFound:
		notFound(w)
	default:
		unavailable(err, w)
	}
}

func Register(root string, db *bolt.DB) error {
	goji.Get(fmt.Sprintf("%s", root), listClients)
	// goji.Post(fmt.Sprintf("%s", root), createClient)
	goji.Get(fmt.Sprintf("%s/:token", root), showClient)
	goji.Post(fmt.Sprintf("%s/:token/permissions", root), addPermissions)
	goji.Delete(fmt.Sprintf("%s/:token/permissions", root), removePermissions)
	// goji.Delete(fmt.Sprintf("%s/:token", root), revokeClient)
	return nil
}
