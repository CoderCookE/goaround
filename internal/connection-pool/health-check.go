package connectionpool

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

type healthCheckReponse struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

type healthChecker struct {
	subscribers    []chan bool
	current_health bool
	client         *http.Client
	backend        string
}

func (hc *healthChecker) Start() {
	ticker := time.NewTicker(1000 * time.Millisecond)
	for _ = range ticker.C {
		hc.healthCheck()
	}
}

func (hc *healthChecker) healthCheck() {
	url := fmt.Sprintf("%s%s", hc.backend, "/health")

	healthy := hc.current_health

	if resp, err := hc.client.Get(url); err != nil {
		log.Printf("Error with health check, backend: %s, error %s", hc.backend, err.Error())
		healthy = false
	} else {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Err with health check, backend: %s, error %s", hc.backend, err.Error())
			healthy = false
		} else {
			defer resp.Body.Close()

			healthCheck := &healthCheckReponse{}
			json.Unmarshal(body, healthCheck)

			healthy = healthCheck.State == "healthy"
		}
	}

	if healthy != hc.current_health {
		hc.current_health = healthy
		for _, c := range hc.subscribers {
			c <- healthy
		}
	}
}
