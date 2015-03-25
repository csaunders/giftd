package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"syscall"
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

func writePidfile(pidfile string) {
	if len(pidfile) > 0 {
		file, err := os.OpenFile(pidfile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		pid := syscall.Getpid()
		_, err = file.Write([]byte(fmt.Sprintf("%d\n", pid)))
		if err != nil {
			log.Fatal(err)
		}
	}
}

func writeAdminToken(token string) {
	file, err := os.OpenFile("admin.token", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	_, err = file.Write([]byte(token))
	if err != nil {
		log.Fatal(err)
	}
}

func initialize() error {
	var dataDir string
	var pidfile string
	flag.StringVar(&dataDir, "datadir", "/var/lib/giftd", "Location where giftd data should be stored")
	flag.StringVar(&pidfile, "pidfile", "", "Location to write pidfile")
	flag.Parse()

	writePidfile(pidfile)
	return os.Chdir(dataDir)
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
		writeAdminToken(token)
	}
}

func main() {
	if err := initialize(); err != nil {
		log.Fatal(err)
	}
	setupPermissionsDb()
	db := dbConnect("giftd.db")
	defer db.Close()

	if err := gifs.Register("/gifs", db); err != nil {
		log.Fatal(err)
	}

	goji.Use(APIAccessManagement)
	goji.Serve()
}
