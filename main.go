package main

import (
	"fmt"
	"log"
	"time"

	"github.com/boltdb/bolt"
	"github.com/zenazn/goji"

	"github.com/csaunders/giftd/gifs"
)

const gifsDatabase string = "giftd.db"
const gifsConfigDb string = "giftd-config.db"

var permissions map[string]string = map[string]string{
	`/gifs/[a-z]+/random`:             "public",
	`/gifs/.{8}-.{4}-.{4}-.{4}-.{12}`: "public",
	`/gifs.*`:                         "gifs-api",
}

func dbConnect(name string) *bolt.DB {
	db, err := bolt.Open(name, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)
	}
	return db
}

func setupPermissionsDb() {
	db := dbConnect(gifsConfigDb)
	defer db.Close()
	for path, scope := range permissions {
		err := setPermissions(db, path, scope)
		if err != nil {
			log.Fatal(err)
		}
	}
	hasAdminToken, err := HasAdministratorToken(db)
	if err != nil {
		log.Fatal("has admin token:", err)
	}

	if !hasAdminToken {
		opts := TokenOptions{Permissions: "admin"}
		token, err := GenerateToken(db, opts)
		if err != nil {
			log.Fatal("generate token:", err)
		}
		fmt.Println("Administrator API Token:", token)
	}
}

func main() {
	setupPermissionsDb()
	db := dbConnect("giftd.db")
	defer db.Close()

	if err := gifs.Register("/gifs", db); err != nil {
		log.Fatal(err)
	}

	goji.Use(APIAccessManagement)
	goji.Serve()
}
