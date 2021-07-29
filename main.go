package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"gopkg.in/yaml.v2"
)

var (
	infoLogger  *log.Logger
	fatalLogger *log.Logger
)

type ServerConf struct {
	Address string `yaml:"Address"`
}

func checkFatalError(err error, stage string) {
	if err != nil {
		fatalLogger.Fatalf("@%s: %v\n", stage, err)
	}
}

func (c *ServerConf) getConfig(filename string) error {
	yamlFile, err := ioutil.ReadFile(filename)
	checkFatalError(err, "READING CONFIG FILE")

	err = yaml.Unmarshal(yamlFile, c)
	checkFatalError(err, "PARSING CONFIG FILE")

	return nil
}

func handleClient(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case "POST":
		reqBody, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read the email request", 500)
			return
		}
		fmt.Printf("%s\n", reqBody)
		w.Write([]byte("Received it!"))
	default:
		http.Error(w, "Invalid request", http.StatusNotImplemented)
	}
}

func main() {
	http.HandleFunc("/", handleClient)
	http.ListenAndServe("localhost:8082", nil)
}
