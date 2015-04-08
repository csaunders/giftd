package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/csaunders/giftd/models"
	"github.com/zenazn/goji/web"
)

func deny(w http.ResponseWriter) {
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("Access Denied"))
}

func HasAdministratorToken(db *bolt.DB) (bool, error) {
	hasAdmin := false
	err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := models.ApiClientsBucket(tx)
		if err != nil {
			return err
		}

		bucket.ForEach(func(token, data []byte) error {
			fmt.Println(string(data))
			if hasAdmin = strings.Contains(string(data), "admin"); hasAdmin {
				return errors.New("")
			}
			return nil
		})
		return nil
	})
	return hasAdmin, err
}

func generateAdministrator(db *bolt.DB) (string, error) {
	account, err := models.NewAccount()
	if err != nil {
		return "", err
	}

	account.AddPermissions([]string{"admin"})
	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := models.ApiClientsBucket(tx)
		if err != nil {
			return err
		}
		return models.Save(bucket, account.Token, account)
	})

	if err != nil {
		return "", err
	}
	return account.Token, nil
}

func SetPermissions(db *bolt.DB, path, scope string) error {
	_, err := regexp.Compile(path)
	if err != nil {
		return err
	}

	return db.Update(func(tx *bolt.Tx) error {
		bucket, err := models.ApiAccessBucket(tx)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(path), []byte(scope))
	})
}

func CreateAdministrator(db *bolt.DB) (string, error) {
	hasAdminToken, err := HasAdministratorToken(db)
	if err != nil {
		return "", err
	}

	if !hasAdminToken {
		return generateAdministrator(db)
	}
	return "", nil
}

func hasSufficientPermissions(requiredPerms, actualPerms string) bool {
	fmt.Println("reqd:", requiredPerms, "actual:", actualPerms)
	if strings.Contains(requiredPerms, "public") {
		return true
	}

	if strings.Contains(actualPerms, "admin") {
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
		bucket, err := models.ApiClientsBucket(tx)
		if err != nil {
			return err
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
		bucket, err := models.ApiAccessBucket(tx)
		if err != nil {
			return err
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

		db, ok := c.Env["configuration-db"].(*bolt.DB)
		if !ok {
			fmt.Println("No configuration database")
			deny(w)
			return
		}

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
