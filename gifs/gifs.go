package gifs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/gif"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"

	"github.com/boltdb/bolt"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
)

const root string = "giftd-gifs"
const maxRandGif int = 10
const namespacesBucketName string = "namespaces"

type requestError struct {
	Error string `json:"error"`
}

func genUUID() ([]byte, error) {
	urandom, err := os.OpenFile("/dev/urandom", os.O_RDONLY, 0)
	if err != nil {
		return []byte{}, err
	}
	defer urandom.Close()
	b := make([]byte, 16)
	n, err := urandom.Read(b)

	if err != nil {
		return []byte{}, err
	} else if n != len(b) {
		return []byte{}, errors.New("Could not read a sufficient number of bytes")
	}
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	return []byte(uuid), nil
}

func verifyGif(r io.Reader) ([]byte, error) {
	data, err := ioutil.ReadAll(r)
	buff := bytes.NewBuffer(data)
	_, err = gif.Decode(buff)
	if err != nil {
		return []byte{}, err
	}
	return data, err
}

func storeGif(db *bolt.DB, ns, uuid, content []byte) error {
	return db.Update(func(tx *bolt.Tx) error {
		rootBucket := tx.Bucket([]byte(root))

		namespacesBucket, err := rootBucket.CreateBucketIfNotExists([]byte(namespacesBucketName))
		if err != nil {
			return err
		}

		bucketForNamespace, err := rootBucket.CreateBucketIfNotExists(ns)
		if err != nil {
			return err
		}
		if err = rootBucket.Put(uuid, content); err != nil {
			return err
		}
		if err = bucketForNamespace.Put(uuid, []byte("{}")); err != nil {
			return err
		}
		if err = namespacesBucket.Put(ns, []byte("{}")); err != nil {
			return err
		}

		return nil
	})
}

func retrieveAndVerify(r io.Reader) ([]byte, error) {
	src, err := ioutil.ReadAll(r)
	if err != nil {
		return []byte{}, err
	}
	resp, err := http.Get(string(src))
	if err != nil {
		return []byte{}, err
	}
	defer resp.Body.Close()
	return verifyGif(resp.Body)
}

func errorHandler(err error, c web.C, w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusServiceUnavailable)
	fmt.Fprintf(w, "Looks like something be misbehavin!")
}

func notFound(msg string, c web.C, w http.ResponseWriter, r *http.Request) {
	response(
		http.StatusNotFound,
		struct {
			Error string `json:"error"`
		}{msg},
		c,
		w,
		r,
	)
}

func response(code int, body interface{}, c web.C, w http.ResponseWriter, r *http.Request) {
	content, err := json.Marshal(body)
	if err != nil {
		errorHandler(err, c, w, r)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(content)
}

func nRandomIndiciesFor(db *bolt.DB, namespace []byte, num int) []int {
	var indices []int
	indexMap := map[int]bool{}
	namespaceSize := 0
	db.View(func(tx *bolt.Tx) error {
		namespaceBucket := tx.Bucket([]byte(root)).Bucket(namespace)
		if namespaceBucket != nil {
			namespaceSize = namespaceBucket.Stats().KeyN
		}
		return nil
	})
	indices = make([]int, int(math.Min(float64(num), float64(namespaceSize))))
	index := 0
	retries := 0
	for {
		if retries > 100 || index >= len(indices) {
			break
		}
		n := rand.Intn(namespaceSize)
		if !indexMap[n] {
			indices[index] = n
			indexMap[n] = true
			index++
			retries = 0
		} else {
			retries++
		}
	}
	sort.Sort(sort.IntSlice(indices))
	return indices
}

func findRandomGifs(db *bolt.DB, namespace []byte, num int) ([]string, error) {
	indices := nRandomIndiciesFor(db, namespace, num)
	uuids := make([]string, len(indices))

	err := db.View(func(tx *bolt.Tx) error {
		rootBucket := tx.Bucket([]byte(root))
		bucketForNamespace := rootBucket.Bucket(namespace)
		if bucketForNamespace == nil {
			return errors.New("findRandomGifs: bucket does not exist")
		}
		cursor := bucketForNamespace.Cursor()
		index := 0
		position := 0
		for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
			if index >= len(indices) {
				break
			} else if indices[index] == position {
				uuids[index] = string(k)
				index++
			}
			position++
		}
		return nil
	})
	return uuids, err
}

func listNamespaces(db *bolt.DB) func(c web.C, w http.ResponseWriter, r *http.Request) {
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		var body interface{}
		err := db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(root)).Bucket([]byte(namespacesBucketName))
			stats := b.Stats()
			c := b.Cursor()
			results := make([]string, stats.KeyN)
			i := 0

			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				results[i] = string(k)
				i++
			}

			body = struct {
				Categories []string `json:"categories"`
			}{results}
			return nil
		})

		if err != nil {
			errorHandler(err, c, w, r)
			return
		}
		response(http.StatusOK, body, c, w, r)
	}
}

func showGif(db *bolt.DB) func(c web.C, w http.ResponseWriter, r *http.Request) {
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		uuid := c.URLParams["uuid"]
		var content []byte
		err := db.View(func(tx *bolt.Tx) error {
			content = tx.Bucket([]byte(root)).Get([]byte(uuid))
			return nil
		})
		if err != nil {
			errorHandler(err, c, w, r)
			return
		} else if len(content) <= 0 {
			notFound(fmt.Sprintf("%s does not exist", uuid), c, w, r)
			return
		}
		w.Header().Set("Content-Type", "image/gif")
		w.Write(content)
	}
}

func createGif(db *bolt.DB) func(c web.C, w http.ResponseWriter, r *http.Request) {
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		namespace := c.URLParams["namespace"]
		var content []byte
		var err error
		switch c.URLParams["type"] {
		case "gif":
			content, err = verifyGif(r.Body)
		case "link":
			content, err = retrieveAndVerify(r.Body)
		default:
			response(
				http.StatusNotAcceptable,
				requestError{"Invalid or unspecified resource: use gif or link"},
				c, w, r,
			)
			return
		}

		if err != nil {
			response(
				http.StatusUnsupportedMediaType,
				requestError{"Invalid Content"},
				c, w, r,
			)
			return
		}

		uuid, err := genUUID()
		if err != nil {
			errorHandler(err, c, w, r)
			return
		}

		err = storeGif(db, []byte(namespace), uuid, content)
		if err != nil {
			errorHandler(err, c, w, r)
		} else {
			response(
				http.StatusCreated, struct {
					UUID string `json:"uuid"`
				}{string(uuid)},
				c, w, r,
			)
		}
	}
}

func randomGif(db *bolt.DB) func(c web.C, w http.ResponseWriter, r *http.Request) {
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		namespace := c.URLParams["namespace"]
		uuids, err := findRandomGifs(db, []byte(namespace), 1)

		if err != nil {
			errorHandler(err, c, w, r)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/gifs/%s", string(uuids[0])), http.StatusTemporaryRedirect)
	}
}

func randomNumGifs(db *bolt.DB) func(c web.C, w http.ResponseWriter, r *http.Request) {
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		namespace := c.URLParams["namespace"]
		count, err := strconv.ParseInt(c.URLParams["count"], 10, 64)
		if err != nil {
			errorHandler(err, c, w, r)
			return
		}

		if int(count) > maxRandGif {
			response(
				http.StatusNotAcceptable,
				requestError{fmt.Sprintf("Request exceeds maximum random gif count of %d", maxRandGif)},
				c,
				w,
				r,
			)
			return
		}

		uuids, err := findRandomGifs(db, []byte(namespace), int(count))
		paths := make([]string, len(uuids))
		if err != nil {
			errorHandler(err, c, w, r)
			return
		}
		for i, uuid := range uuids {
			paths[i] = fmt.Sprintf("http://localhost:8000/gifs/%s", uuid)
		}
		response(
			http.StatusOK,
			struct {
				Locations []string `json:"locations"`
			}{paths},
			c,
			w,
			r,
		)
	}
}

func reportGif(db *bolt.DB) func(c web.C, w http.ResponseWriter, r *http.Request) {
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		response(http.StatusNotImplemented, requestError{"Not Implemented"}, c, w, r)
	}
}

func createBucket(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(root))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})
}

func Register(root string, db *bolt.DB) error {
	err := createBucket(db)
	if err != nil {
		return err
	}

	goji.Get(fmt.Sprintf("%s", root), listNamespaces(db))

	// Gif Specific
	goji.Get(fmt.Sprintf("%s/:uuid", root), showGif(db))
	goji.Delete(fmt.Sprintf("%s/:uuid/report", root), reportGif(db))

	// Creation / Retrieval
	goji.Post(fmt.Sprintf("%s/:namespace/:type", root), createGif(db))
	goji.Get(fmt.Sprintf("%s/:namespace/random", root), randomGif(db))
	goji.Get(fmt.Sprintf("%s/:namespace/random/:count", root), randomNumGifs(db))
	return nil
}
