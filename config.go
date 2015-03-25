package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/zenazn/goji/web"
)

func InitializeConfiguration(configPath string) (func(c *web.C, h http.Handler) http.Handler, error) {
	var config map[string]interface{}

	file, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(file, &config)
	if err != nil {
		return nil, err
	}

	return configurationMiddleware(config), nil
}

func configurationMiddleware(config map[string]interface{}) func(c *web.C, h http.Handler) http.Handler {
	return func(c *web.C, h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for k, v := range config {
				c.Env[k] = v
			}

			h.ServeHTTP(w, r)
		})
	}
}
