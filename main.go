package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/boltdb/bolt"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"

	"github.com/csaunders/giftd/gifs"
)

func hello(c web.C, w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello %s!", c.URLParams["name"])
}

func dbConnect() *bolt.DB {
	db, err := bolt.Open("giftd.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)
	}
	return db
}

func main() {
	db := dbConnect()
	defer db.Close()

	goji.Get("/hello/:name", hello)
	if err := gifs.Register("/gifs", db); err != nil {
		log.Fatal(err)
	}
	goji.Serve()
}
