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
		id := c.URLParams["id"]
		idsBucket, err := models.ApiClientIdsBucket(tx)
		if err != nil {
			return err
		}

		token := idsBucket.Get([]byte(id))
		if token == nil {
			return models.RecordNotFound
		}

		clientsBucket, err := models.ApiClientsBucket(tx)
		if err != nil {
			return err
		}
		if raw {
			rawData, err = models.LoadRaw(clientsBucket, string(token))
		} else {
			err = models.Load(clientsBucket, string(token), &account)
		}
		return err
	})
	return account, err, rawData
}

func saveClient(db *bolt.DB, client *models.Account) error {
	return db.Update(func(tx *bolt.Tx) error {
		clientIds, err := models.ApiClientIdsBucket(tx)
		if err != nil {
			return err
		}
		bucket, err := models.ApiClientsBucket(tx)
		if err != nil {
			return err
		}

		clientIds.Put([]byte(client.Id), []byte(client.Token))
		return models.Save(bucket, client.Token, client)
	})
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
	return saveClient(db, account)
}

func listClients(c web.C, w http.ResponseWriter, r *http.Request) {
	db, err := retrieveDb(c, w)
	if err != nil {
		return
	}
	clientIds := []string{}
	err = db.View(func(tx *bolt.Tx) error {
		clients, err := models.ApiClientIdsBucket(tx)
		if err != nil {
			return unavailable(err, w)
		}
		clients.ForEach(func(key, value []byte) error {
			clientIds = append(clientIds, string(key))
			return nil
		})
		return nil
	})
	if err == nil {
		data, _ := json.Marshal(struct {
			Ids []string `json:"ids"`
		}{clientIds})
		w.Write(data)
	}
}

func showClient(c web.C, w http.ResponseWriter, r *http.Request) {
	db, err := retrieveDb(c, w)
	if err != nil {
		return
	}
	_, err, clientAccount := findClient(db, c, true)
	switch err {
	case nil:
		w.Write(clientAccount)
	case models.RecordNotFound:
		notFound(w)
	default:
		unavailable(err, w)
	}
}

func updateClient(c web.C, w http.ResponseWriter, r *http.Request) {
	db, err := retrieveDb(c, w)
	if err != nil {
		return
	}
	client, err, _ := findClient(db, c, false)
	if err == nil {
		params := struct {
			Datastore   string   `json:"datastore"`
			Permissions []string `json:"permissions"`
		}{}
		if err = json.NewDecoder(r.Body).Decode(&params); err == nil {
			client.SetDatastore(params.Datastore)
			client.SetPermissions(params.Permissions)
			err = saveClient(db, &client)
		}
	}
	if err == nil {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(""))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
	}
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

func createClient(c web.C, w http.ResponseWriter, r *http.Request) {
	db, err := retrieveDb(c, w)
	if err != nil {
		return
	}
	client, err := models.NewAccount()
	if err == nil {
		err = modifyPermissions(db, r.Body, client, client.AddPermissions)
	}
	if err != nil {
		unavailable(err, w)
		return
	}

	bytes, _ := json.Marshal(client)
	w.Write(bytes)
}

func revokeClient(c web.C, w http.ResponseWriter, r *http.Request) {
	db, err := retrieveDb(c, w)
	if err != nil {
		return
	}
	client, err, _ := findClient(db, c, false)
	if err == nil {
		err = db.Update(func(tx *bolt.Tx) error {
			idsBucket, err := models.ApiClientIdsBucket(tx)
			if err != nil {
				return err
			}
			accounts, err := models.ApiClientsBucket(tx)
			if err != nil {
				return err
			}
			err = idsBucket.Delete([]byte(client.Id))
			if err != nil {
				return err
			}
			return accounts.Delete([]byte(client.Token))
		})
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

func syncIds(c web.C, w http.ResponseWriter, r *http.Request) {
	var db *bolt.DB
	var ok bool
	if db, ok = c.Env[middleware.ConfigurationDB].(*bolt.DB); !ok {
		unavailable(errors.New("Cannot load configuration database"), w)
		return
	}
	db.Update(func(tx *bolt.Tx) error {
		clientsBucket, _ := models.ApiClientsBucket(tx)
		idsBucket, _ := models.ApiClientIdsBucket(tx)
		sync := func(token, record []byte) bool {
			if token == nil {
				return false
			}
			var client models.Account
			if err := json.Unmarshal(record, &client); err != nil {
				return false
			}
			fmt.Println(client.Id, "----->", client.Token)
			if err := idsBucket.Put([]byte(client.Id), []byte(client.Token)); err != nil {
				fmt.Println(err)
			}

			return true
		}
		cursor := clientsBucket.Cursor()
		fmt.Println("Synchronizing...")
		processing := sync(cursor.First())
		for processing {
			processing = sync(cursor.Next())
		}
		return nil
	})
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(""))
}

func Register(root string) {
	goji.Get(fmt.Sprintf("%s/accounts", root), listClients)
	goji.Post(fmt.Sprintf("%s/accounts", root), createClient)
	goji.Post(fmt.Sprintf("%s/accounts/sync", root), syncIds)
	goji.Get(fmt.Sprintf("%s/accounts/:id", root), showClient)
	goji.Put(fmt.Sprintf("%s/accounts/:id", root), updateClient)
	goji.Post(fmt.Sprintf("%s/accounts/:id/permissions", root), addPermissions)
	goji.Delete(fmt.Sprintf("%s/accounts/:id/permissions", root), removePermissions)
	goji.Delete(fmt.Sprintf("%s/accounts/:id", root), revokeClient)
}
