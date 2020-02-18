package connectionpool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
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
	wg            *sync.WaitGroup
}

func NewHealthChecker(client *http.Client, subscribers []chan message, backend string, currentHealth bool) *healthChecker {
	return &healthChecker{
		client:        client,
		subscribers:   subscribers,
		backend:       backend,
		done:          make(chan bool),
		currentHealth: currentHealth,
		wg:            &sync.WaitGroup{},
	}
}

func (hc *healthChecker) Start(startup *sync.WaitGroup) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startup.Done()

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

func (hc *healthChecker) Reuse(newBackend string, proxy *httputil.ReverseProxy) *healthChecker {
	hc.Lock()
	hc.backend = newBackend
	hc.notifySubscribers(false, hc.backend, proxy)
	hc.Unlock()

	return hc
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
		hc.notifySubscribers(healthy, hc.backend, nil)
	}
}

func (hc *healthChecker) notifySubscribers(healthy bool, backend string, proxy *httputil.ReverseProxy) {
	message := message{health: healthy, backend: backend, proxy: proxy, ack: hc.wg}

	hc.wg.Add(len(hc.subscribers))
	for _, c := range hc.subscribers {
		c <- message
	}

	hc.wg.Wait()
}

func (hc *healthChecker) Shutdown() {
	close(hc.done)
}
