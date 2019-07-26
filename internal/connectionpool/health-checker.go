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
	subscribers   []chan bool
	currentHealth bool
	client        *http.Client
	backend       string
	done          chan bool
}

func (hc *healthChecker) Start() {
	ticker := time.NewTicker(1000 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			hc.check()
		case <-hc.done:
			ticker.Stop()
			return
		}
	}
}

func (hc *healthChecker) check() {
	url := fmt.Sprintf("%s%s", hc.backend, "/health")

	healthy := hc.currentHealth

	if resp, err := hc.client.Get(url); err != nil {
		log.Printf("Error with health check, backend: %s, error %s", hc.backend, err.Error())
		healthy = false
	} else {
		body, err := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()

		if err != nil {
			log.Printf("Error with health check, backend: %s, error %s", hc.backend, err.Error())
			healthy = false
		} else {
			healthCheck := &healthCheckReponse{}
			json.Unmarshal(body, healthCheck)

			healthy = healthCheck.State == "healthy" || (resp.StatusCode == 200 && healthCheck.State != "degraded")
		}
	}

	if healthy != hc.currentHealth {
		hc.currentHealth = healthy
		for _, c := range hc.subscribers {
			c <- healthy
		}
	}
}

func (hc *healthChecker) Shutdown() {
	close(hc.done)
}
