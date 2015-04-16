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

	"github.com/csaunders/giftd/admin"
	"github.com/csaunders/giftd/gifs"
	"github.com/csaunders/giftd/middleware"
)

const gifsDatabase string = "giftd.db"
const gifsConfigDb string = "giftd-config.db"
const giftdConfig string = "giftd.json"

var permissions map[string]string = map[string]string{
	`/gifs/[a-z]+/random`:             "public",
	`/gifs/.{8}-.{4}-.{4}-.{4}-.{12}`: "public",
	`/gifs.*`:                         "gifs-api",
	`/admin.*`:                        "admin-api",
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
	if len(token) <= 0 {
		return
	}
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
		err := middleware.SetPermissions(db, path, scope)
		if err != nil {
			log.Fatal(err)
		}
	}
	if token, err := middleware.CreateAdministrator(db); err != nil {
		log.Fatal("middleware.CreateAdministrator:", err)
	} else {
		writeAdminToken(token)
	}
}

func main() {
	if err := initialize(); err != nil {
		log.Fatal(err)
	}
	setupPermissionsDb()
	confDb := dbConnect(gifsConfigDb)
	defer confDb.Close()

	gifs.Register("/gifs", middleware.EnvironmentDatabaseProvider)
	admin.Register("/admin")

	configMiddleware, err := middleware.InitializeConfiguration(giftdConfig, confDb)
	if err != nil {
		fmt.Println(err)
	}

	goji.Use(configMiddleware)
	goji.Use(middleware.APIAccessManagement)
	goji.Use(middleware.DatastoreLoader)
	goji.Serve()
}
