package models

import (
	"errors"
	"fmt"

	"github.com/boltdb/bolt"
)

const protectedApisBucket string = "protected-apis"
const apiClientsBucket string = "api-clients"
const apiClientIdsBucket string = "api-client-ids"

func dump(header string, c *bolt.Cursor) {
	fmt.Println("----------", header, "----------")
	printer := func(k, v []byte) bool {
		if k == nil {
			return false
		}
		fmt.Println(string(k), "------>", string(v))
		return true
	}
	printer(c.First())
	for true {
		if printer(c.Next()) == false {
			break
		}
	}
}

func ApiClientsBucket(tx *bolt.Tx) (*bolt.Bucket, error) {
	if tx.Writable() {
		return tx.CreateBucketIfNotExists([]byte(apiClientsBucket))
	} else {
		bucket := tx.Bucket([]byte(apiClientsBucket))
		if bucket == nil {
			return nil, bucketMissing(apiClientsBucket)
		}
		return bucket, nil
	}
}

func ApiClientIdsBucket(tx *bolt.Tx) (*bolt.Bucket, error) {
	if tx.Writable() {
		return tx.CreateBucketIfNotExists([]byte(apiClientIdsBucket))
	} else {
		bucket := tx.Bucket([]byte(apiClientIdsBucket))
		if bucket == nil {
			return nil, bucketMissing(apiClientIdsBucket)
		}
		return bucket, nil
	}

}

func ApiAccessBucket(tx *bolt.Tx) (*bolt.Bucket, error) {
	if tx.Writable() {
		return tx.CreateBucketIfNotExists([]byte(protectedApisBucket))
	} else {
		bucket := tx.Bucket([]byte(protectedApisBucket))
		if bucket == nil {
			return nil, bucketMissing(protectedApisBucket)
		}
		return bucket, nil
	}
}

func bucketMissing(name string) error {
	return errors.New(fmt.Sprintf("buckets: %s does not exist and transaction is not writable", name))
}
