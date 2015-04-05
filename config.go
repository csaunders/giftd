package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/zenazn/goji/web"
)

func InitializeConfiguration(configPath string, configDb interface{}) (func(c *web.C, h http.Handler) http.Handler, error) {
	config := map[string]interface{}{"configuration-db": configDb}

	err := updateConfiguration(config, configPath)
	return configurationMiddleware(config), err
}

func updateConfiguration(config map[string]interface{}, configPath string) error {
	file, err := ioutil.ReadFile(configPath)
	if err != nil {
		return err
	}

	var unmarshalled map[string]interface{}
	err = json.Unmarshal(file, &unmarshalled)
	if err != nil {
		return err
	}

	for key, value := range unmarshalled {
		config[key] = value
	}
	return nil
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
