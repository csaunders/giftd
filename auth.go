package main

import (
	"crypto/rand"
	"encoding/base64"
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

type TokenOptions struct {
	AccessToken string
	Permissions string
}

func makeToken(size int) (string, error) {
	rb := make([]byte, size)
	_, err := rand.Read(rb)

	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(rb), nil
}

func merge(a, b string) string {
	entriesA := strings.Split(a, ",")
	entriesB := strings.Split(b, ",")
	results := make([]string, len(entriesA)+len(entriesB))
	idx := 0
	for i := 0; i < len(entriesA); i++ {
		results[idx] = entriesA[i]
		idx++
	}
	for i := 0; i < len(entriesB); i++ {
		results[idx] = entriesB[i]
		idx++
	}
	return strings.Join(results, ",")
}

func remove(subject, values string) string {
	perms := []string{}
	initial := strings.Split(subject, ",")
	for _, entry := range initial {
		if !strings.Contains(values, entry) {
			perms = append(perms, entry)
		}
	}
	return strings.Join(perms, ",")
}

func HasAdministratorToken(db *bolt.DB) (bool, error) {
	hasAdmin := false
	err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(protectedApisBucket))
		if err != nil {
			return err
		}

		bucket.ForEach(func(token, perms []byte) error {
			if strings.Contains(string(perms), "admin") {
				hasAdmin = true
				return errors.New("")
			}
			return nil
		})
		return nil
	})
	return hasAdmin, err
}

func GenerateToken(db *bolt.DB, opts TokenOptions) (string, error) {
	token, err := makeToken(64)
	if err != nil {
		return token, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(protectedApisBucket))
		if bucket == nil {
			return nil
		}
		return bucket.Put([]byte(token), []byte(opts.Permissions))
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func RevokeToken(db *bolt.DB, opts TokenOptions) error {
	return db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(protectedApisBucket))
		if bucket == nil {
			return nil
		}
		return bucket.Delete([]byte(opts.AccessToken))
	})
}

func GrantTokenPermissions(db *bolt.DB, opts TokenOptions) error {
	return db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(protectedApisBucket))
		if bucket == nil {
			return nil
		}
		existingPerms := string(bucket.Get([]byte(opts.AccessToken)))
		mergedPerms := merge(opts.Permissions, existingPerms)
		return bucket.Put([]byte(opts.AccessToken), []byte(mergedPerms))
	})
}

func RemoveTokenPermissions(db *bolt.DB, opts TokenOptions) error {
	return db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(protectedApisBucket))
		if bucket == nil {
			return nil
		}
		existingPerms := string(bucket.Get([]byte(opts.AccessToken)))
		remainingPerms := remove(existingPerms, opts.Permissions)
		return bucket.Put([]byte(opts.AccessToken), []byte(remainingPerms))
	})
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
	if strings.Contains(perms, "admin") {
		return true
	}

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
		if c.Env["skipAuth"] != nil {
			h.ServeHTTP(w, r)
			return
		}

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
