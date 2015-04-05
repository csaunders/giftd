package admin

import (
	"encoding/json"
	"errors"
	"fmt"
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

func unavailable(err error, w http.ResponseWriter) {
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte(err.Error()))
}

func retrieveDb(c web.C, w http.ResponseWriter) (*bolt.DB, error) {
	if db, ok := c.Env[middleware.ConfigurationDB].(*bolt.DB); ok {
		return db, nil
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte("retrieveDb: database not available in context"))
	return nil, errors.New("retrieveDb: database not available in context")
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
			unavailable(err, w)
			return err
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

func Register(root string, db *bolt.DB) error {
	goji.Get(fmt.Sprintf("%s", root), listClients)
	// goji.Post(fmt.Sprintf("%s", root), createClient)
	// goji.Get(fmt.Sprintf("%s/:token", root), showClient)
	// goji.Post(fmt.Sprintf("%s/:token/permissions", root), addPermissions)
	// goji.Delete(fmt.Sprintf("%s/:token/permissions", root), removePermissions)
	// goji.Delete(fmt.Sprintf("%s/:token", root), revokeClient)
	return nil
}
