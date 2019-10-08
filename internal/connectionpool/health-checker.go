package connectionpool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"
)

type healthCheckReponse struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

type healthChecker struct {
	sync.Mutex
	subscribers   []chan message
	currentHealth bool
	client        *http.Client
	backend       string
	done          chan bool
}

func (hc *healthChecker) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hc.Lock()
	hc.check(ctx)
	hc.Unlock()

	ticker := time.NewTicker(1000 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			cancel()
			hc.Lock()
			ctx, cancel = context.WithCancel(context.Background())
			hc.check(ctx)
			hc.Unlock()
		case <-hc.done:
			ticker.Stop()
			return
		}
	}
}

func (hc *healthChecker) Reuse(newBackend string) {
	hc.Lock()
	hc.notifySubscribers(false)
	hc.backend = newBackend
	hc.Unlock()
}

func (hc *healthChecker) Remove() {

}

func (hc *healthChecker) check(ctx context.Context) {
	url := fmt.Sprintf("%s%s", hc.backend, "/health")
	healthy := hc.currentHealth

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Printf("Error creating request: %s, error %s", hc.backend, err.Error())
		healthy = false
	} else if resp, err := http.DefaultClient.Do(req.WithContext(ctx)); err != nil {
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
		hc.notifySubscribers(healthy)
	}
}

func (hc *healthChecker) notifySubscribers(healthy bool) {
	message := message{health: healthy}

	for _, c := range hc.subscribers {
		c <- message
	}
}

func (hc *healthChecker) Shutdown() {
	close(hc.done)
}
