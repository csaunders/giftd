package main

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/zenazn/goji/web"
)

const protectedApisBucket string = "protected-apis"

func deny(w http.ResponseWriter) {
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("Access Denied"))
}

func setPermissions(db *bolt.DB, path, scope string) error {
	_, err := regexp.Compile(path)
	if err != nil {
		return err
	}

	return db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(protectedApisBucket))
		if err != nil {
			return err
		}
		return bucket.Put([]byte(path), []byte(scope))
	})
}

func hasSufficientPermissions(requiredPerms, actualPerms string) bool {
	if strings.Contains(requiredPerms, "public") {
		return true
	}
	sufficientPermissions := false
	requiredPermsList := strings.Split(requiredPerms, ",")
	for _, perm := range requiredPermsList {
		if strings.Contains(actualPerms, perm) {
			sufficientPermissions = true
			break
		}
	}
	return sufficientPermissions
}

func permissionsFor(db *bolt.DB, token string) (string, error) {
	perms := ""
	err := db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(protectedApisBucket))
		if bucket == nil {
			return nil
		}
		perms = string(bucket.Get([]byte(token)))
		return nil
	})
	return perms, err
}

func canAccess(db *bolt.DB, path, perms string) bool {
	access := false
	db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(protectedApisBucket))
		if bucket == nil {
			return nil
		}

		bucket.ForEach(func(pathPattern, requiredPerms []byte) error {
			re := regexp.MustCompile(string(pathPattern))
			if re.MatchString(path) {
				access = hasSufficientPermissions(string(requiredPerms), perms)
				if access {
					return errors.New("")
				}
			}
			return nil
		})
		return nil
	})
	return access
}

func APIAccessManagement(c *web.C, h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		db := dbConnect(gifsConfigDb)
		defer db.Close()

		accessToken := r.Header.Get("Authorization")
		perms, err := permissionsFor(db, accessToken)

		if err != nil {
			deny(w)
			return
		}

		if canAccess(db, r.URL.Path, perms) {
			fmt.Println("Access Granted for", r.URL.Path)
			h.ServeHTTP(w, r)
			return
		}

		deny(w)
	}
	return http.HandlerFunc(fn)
}
