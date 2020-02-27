package healthcheck

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

	"github.com/CoderCookE/goaround/internal/connection"
	"github.com/CoderCookE/goaround/internal/stats"
)

type Reponse struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

type HealthChecker struct {
	sync.Mutex
	subscribers   []chan connection.Message
	currentHealth bool
	client        *http.Client
	backend       string
	done          chan bool
	Wg            *sync.WaitGroup
}

func New(client *http.Client, subscribers []chan connection.Message, backend string, currentHealth bool) *HealthChecker {
	return &HealthChecker{
		client:        client,
		subscribers:   subscribers,
		backend:       backend,
		done:          make(chan bool),
		currentHealth: currentHealth,
		Wg:            &sync.WaitGroup{},
	}
}

func (hc *HealthChecker) Start(startup *sync.WaitGroup) {
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

func (hc *HealthChecker) Reuse(newBackend string, proxy *httputil.ReverseProxy) *HealthChecker {
	hc.Lock()
	hc.backend = newBackend
	hc.notifySubscribers(false, hc.backend, proxy)
	hc.Unlock()

	return hc
}

func (hc *HealthChecker) check(ctx context.Context) {
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
			healthCheck := &Reponse{}
			json.Unmarshal(body, healthCheck)

			healthy = healthCheck.State == "healthy" || (resp.StatusCode == 200 && healthCheck.State != "degraded")
		}
	}

	if healthy != hc.currentHealth {
		go updateStates(healthy)
		hc.currentHealth = healthy
		hc.notifySubscribers(healthy, hc.backend, nil)
	}
}

func updateStates(healthy bool) {
	if healthy {
		stats.HealthGauge.WithLabelValues("healthy").Add(1)
	} else {
		stats.HealthGauge.WithLabelValues("healthy").Sub(1)
	}
}

func (hc *HealthChecker) notifySubscribers(healthy bool, backend string, proxy *httputil.ReverseProxy) {
	message := connection.Message{Health: healthy, Backend: backend, Proxy: proxy, Ack: hc.Wg}

	hc.Wg.Add(len(hc.subscribers))
	for _, c := range hc.subscribers {
		c <- message
	}

	hc.Wg.Wait()
}

func (hc *HealthChecker) Shutdown() {
	message := connection.Message{Shutdown: true}

	for _, c := range hc.subscribers {
		c <- message
	}

	close(hc.done)
}
